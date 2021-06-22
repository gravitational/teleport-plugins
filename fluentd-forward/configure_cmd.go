/*
Copyright 2015-2021 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/gravitational/trace"
	"github.com/manifoldco/promptui"
)

type ConfigureCmd struct {
	// Out path and file prefix to put certificates into
	Out string `arg:"true" help:"Output directory" type:"existingdir" required:"true"`

	// Addr is Teleport auth proxy instance address
	Addr string `arg:"true" help:"Teleport auth proxy instance address" type:"string" required:"true" default:"localhost:3025"`

	// CAName CA certificate and key name
	CAName string `arg:"true" help:"CA certificate and key name" required:"true" default:"ca"`

	// ServerName server certificate and key name
	ServerName string `arg:"true" help:"Server certificate and key name" required:"true" default:"server"`

	// ClientName client certificate and key name
	ClientName string `arg:"true" help:"Client certificate and key name" required:"true" default:"client"`

	// Certificate TTL
	TTL time.Duration `help:"Certificate TTL" required:"true" default:"87600h"`

	// DNSNames is a DNS subjectAltNames for server cert
	DNSNames []string `help:"Certificate SAN hosts" default:"localhost"`

	// HostNames is an IP subjectAltNames for server cert
	IP []string `help:"Certificate SAN IPs"`

	// Length is RSA key length
	Length int `help:"Key length" enum:"1024,2048,4096" default:"2048"`

	// CN certificate common name
	CN string `help:"Common name for server cert" default:"localhost"`

	// step holds step number for cli messages
	step int

	// caPath target ca cert and pk
	caPaths []string

	// clientPaths target client cert and pk
	clientPaths []string

	// serverPaths target server cert and pk
	serverPaths []string

	// roleDefPath path to target role definition file
	roleDefPath string

	// fluentdConfPath path to target fluentd configuration file
	fluentdConfPath string

	// confPath path to target plugin configuration file
	confPath string
}

var (
	// maxBigInt is a reader for serial number random
	maxBigInt *big.Int = new(big.Int).Lsh(big.NewInt(1), 128)

	//go:embed tpl/teleport-fluentd-forward-role.yaml.tpl
	roleTpl string

	//go:embed tpl/teleport-fluentd-forward.toml.tpl
	confTpl string

	//go:embed tpl/fluent.conf.tpl
	fluentdConfTpl string

	// notBefore is a certificate NotBefore field value
	notBefore time.Time = time.Now()

	// entity is an entity template used in some certs
	entity pkix.Name = pkix.Name{
		Country: []string{"US"},
	}

	// caCert is a fluentd CA certificate
	caCert x509.Certificate = x509.Certificate{
		NotBefore:             notBefore,
		IsCA:                  true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCRLSign | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// clientCert is a fluentd client certificate
	clientCert x509.Certificate = x509.Certificate{
		Subject:     entity,
		NotBefore:   notBefore,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	// Server CSR
	serverCert x509.Certificate = x509.Certificate{
		Subject:     entity,
		NotBefore:   notBefore,
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
)

const (
	// perms certificate/key file permissions
	perms = 0600

	// passwordLength represents rand password length
	passwordLength = 32

	// roleDefFileName is role definition file name
	roleDefFileName = "teleport-fluentd-forward-role.yaml"

	// fluentdConfFileName is fluentd config file name
	fluentdConfFileName = "fluent.conf"

	// confFileName is plugin configuration file name
	confFileName = "teleport-fluentd-forward.toml"

	// guideURL is getting started guide URL
	guideURL = "https://goteleport.com/setup/guides/fluentd"
)

// Validate fills in missing utility values
func (c *ConfigureCmd) Validate() error {
	c.caPaths = []string{path.Join(c.Out, c.CAName) + ".crt", path.Join(c.Out, c.CAName) + ".key"}
	c.clientPaths = []string{path.Join(c.Out, c.ClientName) + ".crt", path.Join(c.Out, c.ClientName) + ".key"}
	c.serverPaths = []string{path.Join(c.Out, c.ServerName) + ".crt", path.Join(c.Out, c.ServerName) + ".key"}
	c.roleDefPath = path.Join(c.Out, roleDefFileName)
	c.fluentdConfPath = path.Join(c.Out, fluentdConfFileName)
	c.confPath = path.Join(c.Out, confFileName)

	// Append SANs and IPs
	c.appendSANs(&serverCert)

	// Assign CNs
	serverCert.Subject.CommonName = c.CN
	clientCert.Subject.CommonName = c.CN

	return nil
}

// Run runs the generator
func (c *ConfigureCmd) Run() error {
	fmt.Printf("Teleport fluentd-forwarder %v %v\n\n", Version, Sha)

	c.step = 1

	rel, err := os.Getwd()
	if err != nil {
		return trace.Wrap(err)
	}

	// Get password either from STDIN or generated string
	pwd, err := c.getPwd()
	if err != nil {
		return err
	}

	// Generate certificates
	err = c.genCerts(pwd)
	if err != nil {
		return err
	}

	paths := append(append(c.caPaths, c.serverPaths...), c.clientPaths...)
	for i, p := range paths {
		r, err := filepath.Rel(rel, p)

		if err != nil {
			return trace.Wrap(err)
		}

		paths[i] = r
	}

	c.printStep("mTLS Fluentd certificates generated and saved to %v", strings.Join(paths, ", "))

	// Write role definition file
	err = c.writeRoleDef()
	if err != nil {
		return trace.Wrap(err)
	}

	p, err := filepath.Rel(rel, c.roleDefPath)
	if err != nil {
		return trace.Wrap(err)
	}

	c.printStep("Generated sample teleport-fluentd-forward role and user file %v", p)

	// Write fluentd configuration file
	err = c.writeFluentdConf(pwd)
	if err != nil {
		return trace.Wrap(err)
	}

	p, err = filepath.Rel(rel, c.fluentdConfPath)
	if err != nil {
		return trace.Wrap(err)
	}

	c.printStep("Generated sample fluentd configuration file %v", p)

	// Write main configuration file
	err = c.writeConf()
	if err != nil {
		return trace.Wrap(err)
	}

	p, err = filepath.Rel(rel, c.confPath)
	if err != nil {
		return trace.Wrap(err)
	}

	c.printStep("Generated plugin configuration file %v", p)

	fmt.Println()
	fmt.Println("Follow-along with our getting started guide:")
	fmt.Println()
	fmt.Println(guideURL)

	return nil
}

// Generates fluentd certificates
func (c *ConfigureCmd) genCerts(pwd string) error {
	caPK, caCertBytes, err := c.genCertAndPK(caCert, nil, nil)
	if err != nil {
		return trace.Wrap(err)
	}

	serverPK, serverCertBytes, err := c.genCertAndPK(serverCert, &caCert, caPK)
	if err != nil {
		return trace.Wrap(err)
	}

	clientPK, clientCertBytes, err := c.genCertAndPK(clientCert, &caCert, caPK)
	if err != nil {
		return trace.Wrap(err)
	}

	err = c.writeKeyAndCert(c.caPaths, caCertBytes, caPK, "")
	if err != nil {
		return trace.Wrap(err)
	}

	err = c.writeKeyAndCert(c.serverPaths, serverCertBytes, serverPK, pwd)
	if err != nil {
		return trace.Wrap(err)
	}

	err = c.writeKeyAndCert(c.clientPaths, clientCertBytes, clientPK, "")
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// appendSANs appends subjectAltName
func (c *ConfigureCmd) appendSANs(cert *x509.Certificate) error {
	cert.DNSNames = c.DNSNames

	if len(c.IP) == 0 {
		for _, name := range c.DNSNames {
			ips, err := net.LookupIP(name)
			if err != nil {
				return trace.Wrap(err)
			}

			if ips != nil {
				cert.IPAddresses = append(cert.IPAddresses, ips...)
			}
		}
	} else {
		for _, ip := range c.IP {
			cert.IPAddresses = append(cert.IPAddresses, net.ParseIP(ip))
		}
	}

	return nil
}

// getPwd returns password read from STDIN or generates new if no password is provided
func (c *ConfigureCmd) getPwd() (string, error) {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Get password from provided file
		pwdFromStdin, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}

		return string(pwdFromStdin), nil
	}

	// Otherwise, generate random hex token
	bytes := make([]byte, passwordLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return hex.EncodeToString(bytes), nil
}

// writePwd writes generated password to the file
func (c *ConfigureCmd) writeFile(path string, content []byte) error {
	ok := c.askOverwrite(path)
	if !ok {
		return nil
	}

	err := ioutil.WriteFile(path, content, perms)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// printStep prints step with number
func (c *ConfigureCmd) printStep(message string, args ...interface{}) {
	p := append([]interface{}{c.step}, args...)
	fmt.Printf("[%v] "+message+"\n", p...)
	c.step++
}

// renderTemplate renders template to writer
func (c *ConfigureCmd) renderTemplate(content string, pipeline interface{}, w io.Writer) error {
	tpl, err := template.New("template").Parse(content)
	if err != nil {
		return trace.Wrap(err)
	}

	err = tpl.ExecuteTemplate(w, "template", pipeline)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// writeRoleDef writes role definition file
func (c *ConfigureCmd) writeRoleDef() error {
	var b bytes.Buffer

	err := c.renderTemplate(roleTpl, nil, &b)
	if err != nil {
		return trace.Wrap(err)
	}

	return c.writeFile(c.roleDefPath, b.Bytes())
}

// writeFluentdConf writes fluentd config file
func (c *ConfigureCmd) writeFluentdConf(pwd string) error {
	var b bytes.Buffer
	var pipeline = struct {
		CaCertFileName     string
		ServerCertFileName string
		ServerKeyFileName  string
		Pwd                string
	}{
		path.Base(c.caPaths[0]),
		path.Base(c.serverPaths[0]),
		path.Base(c.serverPaths[1]),
		pwd,
	}

	err := c.renderTemplate(fluentdConfTpl, pipeline, &b)
	if err != nil {
		return trace.Wrap(err)
	}

	return c.writeFile(c.fluentdConfPath, b.Bytes())
}

// writeFluentdConf writes fluentd config file
func (c *ConfigureCmd) writeConf() error {
	var b bytes.Buffer
	var pipeline = struct {
		CaPaths     []string
		ClientPaths []string
		Addr        string
	}{c.caPaths, c.clientPaths, c.Addr}

	err := c.renderTemplate(confTpl, pipeline, &b)
	if err != nil {
		return trace.Wrap(err)
	}

	return c.writeFile(c.confPath, b.Bytes())
}

// askOverwrite asks question if the user wants to overwrite specified file if it exists
func (c *ConfigureCmd) askOverwrite(path string) bool {
	_, err := os.Stat(path)
	if !os.IsNotExist(err) {
		return c.yesNo(fmt.Sprintf("Do you want to overwrite %s", path))
	}

	return true
}

// yesNo displays Y/N prompt
func (c *ConfigureCmd) yesNo(message string) bool {
	prompt := promptui.Prompt{
		Label:     message,
		IsConfirm: true,
	}

	result, err := prompt.Run()
	if err != nil {
		return false
	}

	return result == "y"
}
