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

package resources

import (
	"github.com/gravitational/teleport/api/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UserStatus defines the observed state of User
type UserStatus struct {
	ResourceStatus `json:",inline"`
}

// User V2.

// UserSpecV2 defines the desired state of User in a Teleport instance.
// In this version it's a Teleport user specification version 2.
type UserSpecV2 types.UserSpecV2

//+kubebuilder:object:root=true

// User is the Schema for the users API
type UserV2 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              UserSpecV2 `json:"spec,omitempty"`
	Status            UserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// UserListV2 contains a list of UserV2
type UserListV2 struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UserV2 `json:"items"`
}

// Marshal serializes a spec into binary data.
func (spec *UserSpecV2) Marshal() ([]byte, error) {
	return (*types.UserSpecV2)(spec).Marshal()
}

// Unmarshal deserializes a spec from binary data.
func (spec *UserSpecV2) Unmarshal(data []byte) error {
	return (*types.UserSpecV2)(spec).Unmarshal(data)
}

// DeepCopyInto deep-copies one user spec into another.
// Required to satisfy runtime.Object interface.
func (spec *UserSpecV2) DeepCopyInto(out *UserSpecV2) {
	data, err := spec.Marshal()
	if err != nil {
		panic(err)
	}
	*out = UserSpecV2{}
	if err = out.Unmarshal(data); err != nil {
		panic(err)
	}
}

// SetErrorStatus sets an error status of a user object.
func (user *UserV2) SetErrorStatus(err error) {
	user.Status.ResourceStatus.SetLastError(err)
}

// ToTeleportUser converts a Kubernetes resource into a Teleport user.
func (user *UserV2) ToTeleportUser() types.User {
	return &types.UserV2{
		Kind:     types.KindRole,
		Version:  types.V2,
		Metadata: types.Metadata{Name: user.Name},
		Spec:     types.UserSpecV2(user.Spec),
	}
}

// Register user types

func init() {
	register(
		&UserV2{},
		&UserListV2{},
	)
}
