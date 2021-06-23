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
	"encoding/pem"
	"io/ioutil"
	"time"

	"github.com/gravitational/trace"
)

// genCertAndPK generates and returns certificate and primary key
func (c *ConfigureCmd) genCertAndPK(cert x509.Certificate, parent *x509.Certificate, signer *rsa.PrivateKey) (*rsa.PrivateKey, []byte, error) {
	sn, err := rand.Int(rand.Reader, maxBigInt)
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}

	cert.SerialNumber = sn                // Assign generated serial number
	cert.NotAfter = time.Now().Add(c.TTL) // Assign expiration time

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
		p = &cert
	}

	// Generate and sign cert
	certBytes, err := x509.CreateCertificate(rand.Reader, &cert, p, &pk.PublicKey, s)
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}

	return pk, certBytes, nil
}

// writeKeyAndCert writes private key and certificate on disk, returns file names actually written
func (c *ConfigureCmd) writeKeyAndCert(certAndKeyPaths []string, certBytes []byte, pk *rsa.PrivateKey, pwd string) error {
	var err error

	ok := c.askOverwrite(certAndKeyPaths[0])
	if !ok {
		return nil
	}

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
