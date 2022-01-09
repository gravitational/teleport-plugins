/*
Copyright 2021 Gravitational, Inc.

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

package certs

import (
	"crypto/x509"
	"encoding/pem"

	"golang.org/x/crypto/ssh"

	"github.com/gravitational/teleport/api/identityfile"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

// CAs contains sets of Teleport cert authorities in a parsed form.
type CAs struct {
	keys map[types.CertAuthType]*CAKeys
}

// CAKeySet is a composite of TLS and SSH sets in a parsed form.
type CAKeySet struct {
	// TLS is a pool of TLS certificates contained in cert authoroity.
	TLS *x509.CertPool

	// SSH is a list of public keys contained in cert authority.
	SSH []ssh.PublicKey
}

// CAKeys contains all keys contained in CA.
type CAKeys struct {
	// Active is an actual set of keys used to sign new certificates and verify them.
	Active *CAKeySet

	// Additional is a set of keys used only to verify certs.
	// Typically, it's non-empty if only the rotation is in progress.
	Additional *CAKeySet
}

// Certs is a composite of TLS and SSH certificates in a parsed form.
// Typically, this is a result of parsing an identity file.
type Certs struct {
	// TLS is a parsed TLS certificate.
	TLS *x509.Certificate

	// SSH is a parsed TLS certificate.
	SSH *ssh.Certificate
}

// ParseCAs decodes the TLS and SSH certs from the cert authorities list to a form suitable for verification of
// signed certificates.
func ParseCAs(authorities []types.CertAuthority) (CAs, error) {
	cas := CAs{make(map[types.CertAuthType]*CAKeys)}
	for _, ca := range authorities {
		caType := ca.GetType()

		// We ignore JWT records because they're not used to issue certificates.
		if caType == types.JWTSigner {
			continue
		}

		keys, ok := cas.keys[caType]
		if !ok {
			keys = &CAKeys{Active: &CAKeySet{TLS: x509.NewCertPool()}, Additional: &CAKeySet{TLS: x509.NewCertPool()}}
			cas.keys[caType] = keys
		}

		activeSet, additionalSet := ca.GetActiveKeys(), ca.GetAdditionalTrustedKeys()

		// Parse TLS key pairs.
		if ok, err := parseTLSKeyPairs(activeSet.TLS, keys.Active.TLS); err != nil {
			return CAs{}, trace.Wrap(err)
		} else if !ok {
			return CAs{}, trace.Errorf("no active tls keypair in %s", ca.GetID())
		}
		if _, err := parseTLSKeyPairs(additionalSet.TLS, keys.Additional.TLS); err != nil {
			return CAs{}, trace.Wrap(err)
		}

		// Parse SSH key pairs.
		pkeys, err := parseSSHKeyPairs(activeSet.SSH)
		if err != nil {
			return CAs{}, trace.Wrap(err)
		}
		if len(pkeys) == 0 {
			return CAs{}, trace.Errorf("no Active.SSH keypair in %s", ca.GetID())
		}
		keys.Active.SSH = append(keys.Active.SSH, pkeys...)

		pkeys, err = parseSSHKeyPairs(additionalSet.SSH)
		if err != nil {
			return CAs{}, trace.Wrap(err)
		}
		keys.Additional.SSH = append(keys.Additional.SSH, pkeys...)
	}
	return cas, nil
}

// ParseIdentity decodes TLS and SSH certs from the identity file contents.
func ParseIdentity(identityFile *identityfile.IdentityFile) (Certs, error) {
	// Parse the TLS certificate.
	pemBlock, _ := pem.Decode(identityFile.Certs.TLS)
	if pemBlock.Type != "CERTIFICATE" {
		return Certs{}, trace.BadParameter("failed to parse tls certificate: bad block type %s", pemBlock.Type)
	}
	tlsCert, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		return Certs{}, trace.Wrap(err, "failed to parse tls certificate")
	}

	// Parse the SSH certificate.
	sshPublicKey, _, _, _, err := ssh.ParseAuthorizedKey(identityFile.Certs.SSH)
	if err != nil {
		return Certs{}, trace.Wrap(err, "failed to parse ssh certificate")
	}
	sshCert, ok := sshPublicKey.(*ssh.Certificate)
	if !ok {
		return Certs{}, trace.BadParameter("failed to parse ssh certificate: not a certificate")
	}

	return Certs{TLS: tlsCert, SSH: sshCert}, nil
}

func parseTLSKeyPairs(keyPairs []*types.TLSKeyPair, out *x509.CertPool) (bool, error) {
	var ok bool
	for _, keyPair := range keyPairs {
		pemBlock, _ := pem.Decode(keyPair.Cert)
		if pemBlock.Type != "CERTIFICATE" {
			return false, trace.Errorf("tls: bad block type %s", pemBlock.Type)
		}

		cert, err := x509.ParseCertificate(pemBlock.Bytes)
		if err != nil {
			return false, trace.Wrap(err)
		}

		out.AddCert(cert)
		ok = true
	}
	return ok, nil
}

func parseSSHKeyPairs(keyPairs []*types.SSHKeyPair) ([]ssh.PublicKey, error) {
	pkeys := make([]ssh.PublicKey, 0, len(keyPairs))
	for _, keyPair := range keyPairs {
		publicKey, _, _, _, err := ssh.ParseAuthorizedKey(keyPair.PublicKey)
		if err != nil {
			return nil, trace.Wrap(err, "failed to parse CA public key")
		}
		pkeys = append(pkeys, publicKey)
	}
	return pkeys, nil
}
