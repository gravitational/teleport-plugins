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
	"io/ioutil"
	"math/big"
	"net"
	"time"

	"github.com/gravitational/trace"
)

type GenCertsCmd struct {
	// Out path and file prefix to put certificates into
	Out string `arg:"true" help:"Output directory and file prefix" required:"true" type:"existingdir"`

	// Pwd key passphrase
	Pwd string `arg:"true" help:"Passphrase" required:"true"`

	// Certificate TTL
	TTL time.Duration `help:"Certificate TTL" required:"true" default:"87600h"`

	// Hosts is a subjectAltNames for server cert
	Hosts []string `help:"Certificate SAN hosts" default:"localhost"`

	// HostNames is a subjectAltNames for server cert
	IP []string `help:"Certificate SAN IPs"`

	// Length is RSA key length
	Length int `help:"Key length" enum:"1024,2048,4096" default:"2048"`

	// CN certificate common name
	CN string `help:"Certificate common name" default:"localhost"`
}

// Run runs the generator
func (c *GenCertsCmd) Run() error {
	// entity := pkix.Name{
	// 	CommonName:   c.CN,
	// 	Country:      []string{"US"},
	// 	Organization: []string{c.CN},
	// }

	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(2019),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(c.TTL),
		Subject:               pkix.Name{CommonName: "ca"},
		IsCA:                  true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCRLSign | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Server key
	// ca.DNSNames = c.Hosts
	// if len(c.IP) == 0 {
	// 	ips, _ := net.LookupIP("localhost")
	// 	if ips != nil {
	// 		ca.IPAddresses = append(ips, net.ParseIP("::1"))
	// 	}
	// } else {
	// 	for ip, _ := range c.IP {
	// 		ca.IPAddresses = append(ca.IPAddresses, net.ParseIP(ip))
	// 	}
	// }

	caPrivKey, err := rsa.GenerateKey(rand.Reader, c.Length)
	if err != nil {
		return trace.Wrap(err)
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return err
	}

	caBytesPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caBytes})
	caPkBytesPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey)})

	err = ioutil.WriteFile("example/keys/ca.crt", caBytesPEM, 0444)
	if err != nil {
		return trace.Wrap(err)
	}

	err = ioutil.WriteFile("example/keys/ca.key", caPkBytesPEM, 0444)
	if err != nil {
		return trace.Wrap(err)
	}

	// SERVER

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1658),
		Subject:      pkix.Name{CommonName: "server"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		//SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	cert.DNSNames = c.Hosts
	if len(c.IP) == 0 {
		ips, _ := net.LookupIP("localhost")
		if ips != nil {
			cert.IPAddresses = append(ips, net.ParseIP("::1"))
		}
	} else {
		for _, ip := range c.IP {
			cert.IPAddresses = append(cert.IPAddresses, net.ParseIP(ip))
		}
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return err
	}

	certBytesPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	certPkBytesPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey)})

	err = ioutil.WriteFile("example/keys/server.crt", certBytesPEM, 0444)
	if err != nil {
		return trace.Wrap(err)
	}

	err = ioutil.WriteFile("example/keys/server.key", certPkBytesPEM, 0444)
	if err != nil {
		return trace.Wrap(err)
	}

	// client

	client := &x509.Certificate{
		SerialNumber: big.NewInt(16538),
		Subject:      pkix.Name{CommonName: "client"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		//SubjectKeyId: []byte{1, 2, 3, 4, 6, 9},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	clientPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	clientBytes, err := x509.CreateCertificate(rand.Reader, client, ca, &clientPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return err
	}

	clientBytesPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientBytes})
	clientPkBytesPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientPrivKey)})

	err = ioutil.WriteFile("example/keys/client.crt", clientBytesPEM, 0444)
	if err != nil {
		return trace.Wrap(err)
	}

	err = ioutil.WriteFile("example/keys/client.key", clientPkBytesPEM, 0444)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}
