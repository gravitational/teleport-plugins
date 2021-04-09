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

package sshutils

import (
	"github.com/gravitational/teleport/api/types"

	"github.com/gravitational/trace"
	"golang.org/x/crypto/ssh"
)

// GetCheckers returns public keys that can be used to check cert authorities
func GetCheckers(ca types.CertAuthority) ([]ssh.PublicKey, error) {
	out := make([]ssh.PublicKey, 0, len(ca.GetCheckingKeys()))
	for _, keyBytes := range ca.GetCheckingKeys() {
		key, _, _, _, err := ssh.ParseAuthorizedKey(keyBytes)
		if err != nil {
			return nil, trace.BadParameter("invalid authority public key (len=%d): %v", len(keyBytes), err)
		}
		out = append(out, key)
	}
	return out, nil
}

// GetSigners returns a list of signers that could be used to sign keys.
func GetSigners(ca types.CertAuthority) ([]ssh.Signer, error) {
	out := make([]ssh.Signer, 0, len(ca.GetSigningKeys()))
	for _, keyBytes := range ca.GetSigningKeys() {
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		signer = AlgSigner(signer, GetSigningAlgName(ca))
		out = append(out, signer)
	}
	return out, nil
}

// GetSigningAlgName returns the CA's signing algorithm type
func GetSigningAlgName(ca types.CertAuthority) string {
	switch ca.GetSigningAlg() {
	// UNKNOWN algorithm can come from a cluster that existed before SigningAlg
	// field was added. Default to RSA-SHA1 to match the implicit algorithm
	// used in those clusters.
	case types.CertAuthoritySpecV2_RSA_SHA1, types.CertAuthoritySpecV2_UNKNOWN:
		return ssh.SigAlgoRSA
	case types.CertAuthoritySpecV2_RSA_SHA2_256:
		return ssh.SigAlgoRSASHA2256
	case types.CertAuthoritySpecV2_RSA_SHA2_512:
		return ssh.SigAlgoRSASHA2512
	default:
		return ""
	}
}

// SetSigningAlgName sets the signing algorithm type for the given CA
func SetSigningAlgName(ca types.CertAuthority, alg string) {
	ca.SetSigningAlg(ParseSigningAlg(alg))
}

// ParseSigningAlg converts the name of the SSH signature algorithm to the
// corresponding proto enum value.
//
// alg should be one of ssh.SigAlgo* constants. If it's not one of those
// constants, types.CertAuthoritySpecV2_UNKNOWN is returned.
func ParseSigningAlg(alg string) types.CertAuthoritySpecV2_SigningAlgType {
	switch alg {
	case ssh.SigAlgoRSA:
		return types.CertAuthoritySpecV2_RSA_SHA1
	case ssh.SigAlgoRSASHA2256:
		return types.CertAuthoritySpecV2_RSA_SHA2_256
	case ssh.SigAlgoRSASHA2512:
		return types.CertAuthoritySpecV2_RSA_SHA2_512
	default:
		return types.CertAuthoritySpecV2_UNKNOWN
	}
}
