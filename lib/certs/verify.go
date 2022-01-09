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
	"bytes"
	"crypto/x509"
	"time"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

// Verification is a result of verifying the certificate.
type Verification struct {
	// KeySet is a key set against which the cert is considered valid.
	// One can compare it with Active/Additional pointers of the CAKeys struct.
	// If it's equal to Additional, then Active key set is failed to verify the cert.
	KeySet *CAKeySet

	// Validity indicates how long certificates will be valid.
	Validity time.Duration
}

// GetKeys returns CA key set for a given CA type.
func (cas CAs) GetKeys(certType types.CertAuthType) (CAKeys, error) {
	keys, ok := cas.keys[certType]
	if !ok {
		return CAKeys{}, trace.Errorf("key set with type %s is not found", certType)
	}
	return *keys, nil
}

// VerifyCerts validates certificates against the CA key set.
func (keys CAKeys) VerifyCerts(certs Certs) (Verification, error) {
	var result Verification
	if err1 := verify(certs, *keys.Active); err1 == nil {
		result.KeySet = keys.Active
	} else if err2 := verify(certs, *keys.Additional); err2 == nil {
		result.KeySet = keys.Additional
	} else {
		return Verification{}, trace.NewAggregate(
			trace.Wrap(err1, "checked against active key set"),
			trace.Wrap(err2, "checked against additional trusted key set"),
		)
	}
	result.Validity = time.Until(time.Unix(int64(certs.SSH.ValidBefore), 0))
	return result, nil
}

func verify(certs Certs, keySet CAKeySet) error {
	// Verify TLS certificate.
	if chains, err := certs.TLS.Verify(x509.VerifyOptions{Roots: keySet.TLS}); err != nil {
		return trace.Wrap(err, "failed to verify tls certificate")
	} else if len(chains) == 0 {
		return trace.Errorf("failed to verify tls certificate: no chain has been built")
	}

	// Verify SSH certificate.
	sigKeyBytes := certs.SSH.SignatureKey.Marshal()
	for _, publicKey := range keySet.SSH {
		if bytes.Equal(publicKey.Marshal(), sigKeyBytes) {
			return nil
		}
	}
	return trace.Errorf("failed to verify ssh certificate")
}
