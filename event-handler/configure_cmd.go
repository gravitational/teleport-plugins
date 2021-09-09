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
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gravitational/trace"

	"github.com/gravitational/teleport-plugins/event-handler/lib"
)

// ConfigureCmd represents configure command behaviour.
//
// teleport-event-handler configure .
//
// It generates fluentd mTLS certificates, example configuration and teleport user/role definitions.
//
// mTLS certificates are self-signed by our own generated CA. So, we generate three certificates: CA, server
// which is used on the fluentd side, and client which is used by the plugin to connect to a fluentd instance.
//
// Please, check README.md for additional info.
type ConfigureCmd struct {
	*ConfigureCmdConfig

	// step holds step number for cli messages
	step int

	// caPath is a path to a target fluentd mTLS CA cert and pk
	caPaths []string

	// clientPaths is a paths to a target mTLS client cert and pk (used by a plugin)
	clientPaths []string

	// serverPaths is a paths to a target mTLS target server cert and pk (used by a fluentd instance)
	serverPaths []string

	// roleDefPath path to target role definition file which contains plugin role and user
	roleDefPath string

	// fluentdConfPath path to target fluentd configuration file which contains an example fluentd configuration
	fluentdConfPath string

	// confPath path to target plugin configuration file which contains an example plugin configuration
	confPath string

	// caCert is a fluentd CA certificate struct used to generate mTLS CA certs
	caCert x509.Certificate

	// clientCert is a fluentd server certificate struct used to generate mTLS fluentd certs
	serverCert x509.Certificate

	// clientCert is a fluentd client certificate struct used by generator mTLS plugin certs
	clientCert x509.Certificate
}

var (
	// maxBigInt is serial number random max
	maxBigInt *big.Int = new(big.Int).Lsh(big.NewInt(1), 128)

	//go:embed tpl/teleport-event-handler-role.yaml.tpl
	roleTpl string

	//go:embed tpl/teleport-event-handler.toml.tpl
	confTpl string

	//go:embed tpl/fluent.conf.tpl
	fluentdConfTpl string
)

const (
	// perms certificate/key file permissions
	perms = 0600

	// passwordLength represents rand password length
	passwordLength = 32

	// roleDefFileName is role definition file name
	roleDefFileName = "teleport-event-handler-role.yaml"

	// fluentdConfFileName is fluentd config file name
	fluentdConfFileName = "fluent.conf"

	// confFileName is plugin configuration file name
	confFileName = "teleport-event-handler.toml"

	// guideURL is getting started guide URL
	guideURL = "https://goteleport.com/docs/setup/guides/forward-events"
)

// RunConfigureCmd initializes and runs configure command
func RunConfigureCmd(cfg *ConfigureCmdConfig) error {
	notBefore := time.Now()
	notAfter := notBefore.Add(cfg.TTL)

	entity := pkix.Name{
		Country:    []string{"US"},
		CommonName: cfg.CN,
	}

	c := ConfigureCmd{
		ConfigureCmdConfig: cfg,
		caPaths:            []string{path.Join(cfg.Out, cfg.CAName) + ".crt", path.Join(cfg.Out, cfg.CAName) + ".key"},
		clientPaths:        []string{path.Join(cfg.Out, cfg.ClientName) + ".crt", path.Join(cfg.Out, cfg.ClientName) + ".key"},
		serverPaths:        []string{path.Join(cfg.Out, cfg.ServerName) + ".crt", path.Join(cfg.Out, cfg.ServerName) + ".key"},
		roleDefPath:        path.Join(cfg.Out, roleDefFileName),
		fluentdConfPath:    path.Join(cfg.Out, fluentdConfFileName),
		confPath:           path.Join(cfg.Out, confFileName),
		caCert: x509.Certificate{ // caCert is a fluentd CA certificate
			NotBefore:             notBefore,
			NotAfter:              notAfter,
			IsCA:                  true,
			MaxPathLenZero:        true,
			KeyUsage:              x509.KeyUsageCRLSign | x509.KeyUsageCertSign,
			BasicConstraintsValid: true,
		},
		clientCert: x509.Certificate{ // clientCert is a fluentd client certificate
			Subject:     entity,
			NotBefore:   notBefore,
			NotAfter:    notAfter,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			KeyUsage:    x509.KeyUsageDigitalSignature,
		},
		serverCert: x509.Certificate{ // Server CSR
			Subject:     entity,
			NotBefore:   notBefore,
			NotAfter:    notAfter,
			KeyUsage:    x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		},
	}

	// Generate and assign serial numbers
	sn, err := rand.Int(rand.Reader, maxBigInt)
	if err != nil {
		return trace.Wrap(err)
	}

	c.caCert.SerialNumber = sn

	sn, err = rand.Int(rand.Reader, maxBigInt)
	if err != nil {
		return trace.Wrap(err)
	}

	c.clientCert.SerialNumber = sn

	sn, err = rand.Int(rand.Reader, maxBigInt)
	if err != nil {
		return trace.Wrap(err)
	}

	c.serverCert.SerialNumber = sn

	// Append SANs and IPs
	c.appendSANs(&c.serverCert)

	return c.Run()
}

// Run runs the generator
func (c *ConfigureCmd) Run() error {
	fmt.Printf("Teleport event handler %v %v\n\n", Version, Sha)

	c.step = 1

	rel, err := os.Getwd()
	if err != nil {
		return trace.Wrap(err)
	}

	// Get password either from STDIN or generated string
	pwd, err := c.getPwd()
	if err != nil {
		return trace.Wrap(err)
	}

	// Generate certificates and save them to desired locations
	err = c.genCerts(pwd)
	if err != nil {
		return trace.Wrap(err)
	}

	// Print paths to generated fluentd certificate files
	paths := append(append(c.caPaths, c.serverPaths...), c.clientPaths...)
	for i, p := range paths {
		r, err := filepath.Rel(rel, p)

		if err != nil {
			return trace.Wrap(err)
		}

		paths[i] = filepath.Clean(r)
	}

	c.printStep("Generated mTLS Fluentd certificates %v", strings.Join(paths, ", "))

	// Write role definition file
	err = c.writeRoleDef()
	if err != nil {
		return trace.Wrap(err)
	}

	p, err := filepath.Rel(rel, c.roleDefPath)
	if err != nil {
		return trace.Wrap(err)
	}

	c.printStep("Generated sample teleport-event-handler role and user file %v", filepath.Clean(p))

	// Write fluentd configuration file
	err = c.writeFluentdConf(pwd)
	if err != nil {
		return trace.Wrap(err)
	}

	p, err = filepath.Rel(rel, c.fluentdConfPath)
	if err != nil {
		return trace.Wrap(err)
	}

	c.printStep("Generated sample fluentd configuration file %v", filepath.Clean(p))

	// Write main configuration file
	err = c.writeConf()
	if err != nil {
		return trace.Wrap(err)
	}

	p, err = filepath.Rel(rel, c.confPath)
	if err != nil {
		return trace.Wrap(err)
	}

	c.printStep("Generated plugin configuration file %v", filepath.Clean(p))

	fmt.Println()
	fmt.Println("Follow-along with our getting started guide:")
	fmt.Println()
	fmt.Println(guideURL)

	return nil
}

// Generates fluentd certificates
func (c *ConfigureCmd) genCerts(pwd string) error {
	caPK, caCertBytes, err := c.genCertAndPK(&c.caCert, nil, nil)
	if err != nil {
		return trace.Wrap(err)
	}

	serverPK, serverCertBytes, err := c.genCertAndPK(&c.serverCert, &c.caCert, caPK)
	if err != nil {
		return trace.Wrap(err)
	}

	clientPK, clientCertBytes, err := c.genCertAndPK(&c.clientCert, &c.caCert, caPK)
	if err != nil {
		return trace.Wrap(err)
	}

	ok := c.askOverwrite(c.caPaths[0])
	if ok {
		err = c.writeKeyAndCert(c.caPaths, caCertBytes, caPK, "")
		if err != nil {
			return trace.Wrap(err)
		}
	}

	ok = c.askOverwrite(c.serverPaths[0])
	if ok {
		err = c.writeKeyAndCert(c.serverPaths, serverCertBytes, serverPK, pwd)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	ok = c.askOverwrite(c.clientPaths[0])
	if ok {
		err = c.writeKeyAndCert(c.clientPaths, clientCertBytes, clientPK, "")
		if err != nil {
			return trace.Wrap(err)
		}
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

// writeRoleDef writes role definition file
func (c *ConfigureCmd) writeRoleDef() error {
	var b bytes.Buffer

	err := lib.RenderTemplate(roleTpl, nil, &b)
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

	err := lib.RenderTemplate(fluentdConfTpl, pipeline, &b)
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

	err := lib.RenderTemplate(confTpl, pipeline, &b)
	if err != nil {
		return trace.Wrap(err)
	}

	return c.writeFile(c.confPath, b.Bytes())
}

// askOverwrite asks question if the user wants to overwrite specified file if it exists
func (c *ConfigureCmd) askOverwrite(path string) bool {
	_, err := os.Stat(path)
	if !os.IsNotExist(err) {
		return lib.AskYesNo(fmt.Sprintf("Do you want to overwrite %s", path))
	}

	return true
}

// genCertAndPK generates and returns certificate and primary key
func (c *ConfigureCmd) genCertAndPK(cert *x509.Certificate, parent *x509.Certificate, signer *rsa.PrivateKey) (*rsa.PrivateKey, []byte, error) {
	// Generate PK
	pk, err := rsa.GenerateKey(rand.Reader, c.Length)
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}

	// Check if it's self-signed, assign signer and parent to self
	s := signer
	p := parent

	if s == nil {
		s = pk
	}

	if p == nil {
		p = cert
	}

	// Generate and sign cert
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, p, &pk.PublicKey, s)
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}

	return pk, certBytes, nil
}

// writeKeyAndCert writes private key and certificate on disk, returns file names actually written
func (c *ConfigureCmd) writeKeyAndCert(certAndKeyPaths []string, certBytes []byte, pk *rsa.PrivateKey, pwd string) error {
	var err error

	pkBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(pk)}
	bytesPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes})

	// Encrypt with passphrase
	if pwd != "" {
		//nolint // deprecated, but we still need it to be encrypted because of fluentd requirements
		pkBlock, err = x509.EncryptPEMBlock(rand.Reader, pkBlock.Type, pkBlock.Bytes, []byte(pwd), x509.PEMCipherAES256)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	pkBytesPEM := pem.EncodeToMemory(pkBlock)

	err = ioutil.WriteFile(certAndKeyPaths[0], bytesPEM, perms)
	if err != nil {
		return trace.Wrap(err)
	}

	err = ioutil.WriteFile(certAndKeyPaths[1], pkBytesPEM, perms)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}
