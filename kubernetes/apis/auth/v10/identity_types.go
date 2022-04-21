/*
Copyright 2021-2022 Gravitational, Inc.

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

package v10

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IdentitySpec defines the desired state of Identity object.
type IdentitySpec struct {
	// Username is a name of the Teleport user for whom we generate an identity.
	// +required
	Username string `json:"username"`

	// SecretName is a name of the secret resource to store the Teleport identity contents.
	// Secret resource is located in the same namespace as Identity resource.
	// +required
	SecretName string `json:"secretName"`

	// TTL is a duration of TLS/SSH certificates lifetime.
	// +optional
	TTL *metav1.Duration `json:"ttl"`
}

// IdentityStatus defines the observed state of Identity object.
type IdentityStatus struct {
	// NeedRenewal indicates that identity secret must be re-generated.
	NeedRenewal bool `json:"needRenewal"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Identity is the Schema for the identities API
// Identity resource describes a request for Teleport identity file to be generated.
type Identity struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IdentitySpec   `json:"spec,omitempty"`
	Status IdentityStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IdentityList contains a list of Identity objects.
type IdentityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Identity `json:"items"`
}

func init() {
	schemeBuilder.Register(&Identity{}, &IdentityList{})
}
