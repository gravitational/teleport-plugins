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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"path"
	"time"

	"github.com/gravitational/trace"
	"github.com/manifoldco/promptui"
	log "github.com/sirupsen/logrus"
)

type GenCertsCmd struct {
	// Out path and file prefix to put certificates into
	Out string `arg:"true" help:"Output directory" type:"existingdir" required:"true"`

	// Pwd key passphrase
	Pwd string `arg:"true" help:"Passphrase" required:"true"`

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
}

var (
	// maxBigInt is a reader for serial number random
	maxBigInt *big.Int = new(big.Int).Lsh(big.NewInt(1), 128)
)

const (
	// perms certificate/key file permissions
	perms = 0066
)

// Run runs the generator
func (c *GenCertsCmd) Run() error {
	entity := pkix.Name{
		CommonName: c.CN,
		Country:    []string{"US"},
	}

	// CA CSR
	sn, err := rand.Int(rand.Reader, maxBigInt)
	if err != nil {
		return trace.Wrap(err)
	}

	notBefore := time.Now()
	notAfter := time.Now().Add(c.TTL)

	caCert := &x509.Certificate{
		SerialNumber:          sn,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCRLSign | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Client CSR
	sn, err = rand.Int(rand.Reader, maxBigInt)
	if err != nil {
		return trace.Wrap(err)
	}

	clientCert := &x509.Certificate{
		SerialNumber: sn,
		Subject:      entity,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	sn, err = rand.Int(rand.Reader, maxBigInt)
	if err != nil {
		return trace.Wrap(err)
	}

	// Server CSR
	serverCert := &x509.Certificate{
		SerialNumber: sn,
		Subject:      entity,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	c.appendSANs(serverCert)

	// Generate CA key and certificate
	caPK, err := rsa.GenerateKey(rand.Reader, c.Length)
	if err != nil {
		return trace.Wrap(err)
	}

	caCertBytes, err := x509.CreateCertificate(rand.Reader, caCert, caCert, &caPK.PublicKey, caPK)
	if err != nil {
		return trace.Wrap(err)
	}

	err = c.writeKeyAndCert(path.Join(c.Out, c.CAName), caCertBytes, caPK, "")
	if err != nil {
		return trace.Wrap(err)
	}

	// Generate server key and certificate
	serverPK, err := rsa.GenerateKey(rand.Reader, c.Length)
	if err != nil {
		return trace.Wrap(err)
	}

	serverCertBytes, err := x509.CreateCertificate(rand.Reader, serverCert, caCert, &serverPK.PublicKey, caPK)
	if err != nil {
		return trace.Wrap(err)
	}

	err = c.writeKeyAndCert(path.Join(c.Out, c.ServerName), serverCertBytes, serverPK, c.Pwd)
	if err != nil {
		return trace.Wrap(err)
	}

	// Generate client key and certificate
	clientPK, err := rsa.GenerateKey(rand.Reader, c.Length)
	if err != nil {
		return trace.Wrap(err)
	}

	clientCertBytes, err := x509.CreateCertificate(rand.Reader, clientCert, caCert, &clientPK.PublicKey, caPK)
	if err != nil {
		return trace.Wrap(err)
	}

	err = c.writeKeyAndCert(path.Join(c.Out, c.ClientName), clientCertBytes, clientPK, "")
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// writeKeyAndCert writes private key and certificate on disk
func (c *GenCertsCmd) writeKeyAndCert(prefix string, certBytes []byte, pk *rsa.PrivateKey, pwd string) error {
	crtPath := prefix + ".crt"
	keyPath := prefix + ".key"

	_, err := os.Stat(crtPath)
	if !os.IsNotExist(err) {
		if !yesNo(fmt.Sprintf("Do you want to overwrite %s?", crtPath)) {
			return nil
		}
	}

	pkBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(pk)}
	bytesPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes})

	// Encrypt with passphrase
	if pwd != "" {
		//nolint // deprecated, but we still need it to be encrypted because of fluentd requirements
		pkBlock, err = x509.EncryptPEMBlock(rand.Reader, pkBlock.Type, pkBlock.Bytes, []byte(pwd), x509.PEMCipherAES256)
		if err != nil {
			return nil
		}
	}

	pkBytesPEM := pem.EncodeToMemory(pkBlock)

	err = ioutil.WriteFile(crtPath, bytesPEM, perms)
	if err != nil {
		return trace.Wrap(err)
	}

	err = ioutil.WriteFile(keyPath, pkBytesPEM, perms)
	if err != nil {
		return trace.Wrap(err)
	}

	log.WithField("path", crtPath).Info("Certificate saved")
	log.WithField("key", keyPath).Info("Private key saved")

	return nil
}

// appendSANs appends subjectAltName
func (c *GenCertsCmd) appendSANs(cert *x509.Certificate) error {
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

// yesNo displays Y/N prompt
func yesNo(message string) bool {
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
