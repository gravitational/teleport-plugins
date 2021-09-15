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

// Package sshutils defines several functions and types used across the
// Teleport API and other Teleport packages when working with SSH.
package sshutils

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net"
	"runtime"

	"github.com/gravitational/teleport/api/constants"
	"github.com/gravitational/teleport/api/defaults"

	"github.com/gravitational/trace"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// ParseCertificate parses an SSH certificate from the authorized_keys format.
func ParseCertificate(buf []byte) (*ssh.Certificate, error) {
	k, _, _, _, err := ssh.ParseAuthorizedKey(buf)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	cert, ok := k.(*ssh.Certificate)
	if !ok {
		return nil, trace.BadParameter("not an SSH certificate")
	}

	return cert, nil
}

// ParseKnownHosts parses provided known_hosts entries into ssh.PublicKey list.
func ParseKnownHosts(knownHosts [][]byte) ([]ssh.PublicKey, error) {
	var keys []ssh.PublicKey
	for _, line := range knownHosts {
		for {
			_, _, publicKey, _, bytes, err := ssh.ParseKnownHosts(line)
			if err == io.EOF {
				break
			} else if err != nil {
				return nil, trace.Wrap(err, "failed parsing known hosts: %v; raw line: %q", err, line)
			}
			keys = append(keys, publicKey)
			line = bytes
		}
	}
	return keys, nil
}

// ParseAuthorizedKeys parses provided authorized_keys entries into ssh.PublicKey list.
func ParseAuthorizedKeys(authorizedKeys [][]byte) ([]ssh.PublicKey, error) {
	var keys []ssh.PublicKey
	for _, line := range authorizedKeys {
		publicKey, _, _, _, err := ssh.ParseAuthorizedKey(line)
		if err != nil {
			return nil, trace.Wrap(err, "failed parsing authorized keys: %v; raw line: %q", err, line)
		}
		keys = append(keys, publicKey)
	}
	return keys, nil
}

// ProxyClientSSHConfig returns an ssh.ClientConfig with SSH credentials from this
// Key and HostKeyCallback matching SSH CAs in the Key.
//
// The config is set up to authenticate to proxy with the first available principal.
//
func ProxyClientSSHConfig(sshCert, privKey []byte, caCerts [][]byte) (*ssh.ClientConfig, error) {
	cert, err := ParseCertificate(sshCert)
	if err != nil {
		return nil, trace.Wrap(err, "failed to extract username from SSH certificate")
	}

	authMethod, err := AsAuthMethod(cert, privKey)
	if err != nil {
		return nil, trace.Wrap(err, "failed to convert key pair to auth method")
	}

	hostKeyCallback, err := HostKeyCallback(caCerts, false)
	if err != nil {
		return nil, trace.Wrap(err, "failed to convert certificate authorities to HostKeyCallback")
	}

	// The KeyId is not always a valid principal, so we use the first valid principal instead.
	user := cert.KeyId
	if len(cert.ValidPrincipals) > 0 {
		user = cert.ValidPrincipals[0]
	}

	return &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: hostKeyCallback,
		Timeout:         defaults.DefaultDialTimeout,
	}, nil
}

// AsSigner returns an ssh.Signer from raw marshaled key and certificate.
func AsSigner(sshCert *ssh.Certificate, privKey []byte) (ssh.Signer, error) {
	keys, err := AsAgentKeys(sshCert, privKey)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	signer, err := ssh.NewSignerFromKey(keys[0].PrivateKey)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	signer, err = ssh.NewCertSigner(keys[0].Certificate, signer)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return signer, nil
}

// AsAuthMethod returns an "auth method" interface, a common abstraction
// used by Golang SSH library. This is how you actually use a Key to feed
// it into the SSH lib.
func AsAuthMethod(sshCert *ssh.Certificate, privKey []byte) (ssh.AuthMethod, error) {
	signer, err := AsSigner(sshCert, privKey)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return ssh.PublicKeys(signer), nil
}

// AsAgentKeys converts Key struct to a []*agent.AddedKey. All elements
// of the []*agent.AddedKey slice need to be loaded into the agent!
func AsAgentKeys(sshCert *ssh.Certificate, privKey []byte) ([]agent.AddedKey, error) {
	// unmarshal private key bytes into a *rsa.PrivateKey
	privateKey, err := ssh.ParseRawPrivateKey(privKey)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// put a teleport identifier along with the teleport user into the comment field
	comment := fmt.Sprintf("teleport:%v", sshCert.KeyId)

	// On Windows, return the certificate with the private key embedded.
	if runtime.GOOS == constants.WindowsOS {
		return []agent.AddedKey{
			{
				PrivateKey:       privateKey,
				Certificate:      sshCert,
				Comment:          comment,
				LifetimeSecs:     0,
				ConfirmBeforeUse: false,
			},
		}, nil
	}

	// On Unix, return the certificate (with embedded private key) as well as
	// a private key.
	//
	// This is done because OpenSSH clients older than OpenSSH 7.3/7.3p1
	// (2016-08-01) have a bug in how they use certificates that have been loaded
	// in an agent. Specifically when you add a certificate to an agent, you can't
	// just embed the private key within the certificate, you have to add the
	// certificate and private key to the agent separately. Teleport works around
	// this behavior to ensure OpenSSH interoperability.
	//
	// For more details see the following: https://bugzilla.mindrot.org/show_bug.cgi?id=2550
	// WARNING: callers expect the returned slice to be __exactly as it is__
	return []agent.AddedKey{
		{
			PrivateKey:       privateKey,
			Certificate:      sshCert,
			Comment:          comment,
			LifetimeSecs:     0,
			ConfirmBeforeUse: false,
		},
		{
			PrivateKey:       privateKey,
			Certificate:      nil,
			Comment:          comment,
			LifetimeSecs:     0,
			ConfirmBeforeUse: false,
		},
	}, nil
}

// HostKeyCallback returns an ssh.HostKeyCallback that validates host
// keys/certs against SSH CAs in the Key.
//
// If not CAs are present in the Key, the returned ssh.HostKeyCallback is nil.
// This causes golang.org/x/crypto/ssh to prompt the user to verify host key
// fingerprint (same as OpenSSH does for an unknown host).
func HostKeyCallback(caCerts [][]byte, withHostKeyFallback bool) (ssh.HostKeyCallback, error) {
	trustedKeys, err := ParseKnownHosts(caCerts)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// No CAs are provided, return a nil callback which will prompt the user
	// for trust.
	if len(trustedKeys) == 0 {
		return nil, nil
	}

	callbackConfig := HostKeyCallbackConfig{
		GetHostCheckers: func() ([]ssh.PublicKey, error) {
			return trustedKeys, nil
		},
	}

	if withHostKeyFallback {
		callbackConfig.HostKeyFallback = hostKeyFallbackFunc(trustedKeys)
	}

	callback, err := NewHostKeyCallback(callbackConfig)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return callback, nil
}

func hostKeyFallbackFunc(knownHosts []ssh.PublicKey) func(hostname string, remote net.Addr, key ssh.PublicKey) error {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		for _, knownHost := range knownHosts {
			if KeysEqual(key, knownHost) {
				return nil
			}
		}
		return trace.AccessDenied("host %v presented a public key instead of a host certificate which isn't among known hosts", hostname)
	}
}

// KeysEqual is constant time compare of the keys to avoid timing attacks
func KeysEqual(ak, bk ssh.PublicKey) bool {
	a := ssh.Marshal(ak)
	b := ssh.Marshal(bk)
	return (len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1)
}
