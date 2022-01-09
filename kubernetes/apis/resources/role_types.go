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

// RoleStatus defines the observed state of Role
type RoleStatus struct {
	ResourceStatus `json:",inline"`
}

// Role V4.

// RoleSpecV4 defines the desired state of Role in a Teleport instance.
// In this version it's a Teleport role specification version 4.
type RoleSpecV4 types.RoleSpecV4

//+kubebuilder:object:root=true

// RoleV4 is the Schema for the roles API version 4.
type RoleV4 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RoleSpecV4 `json:"spec,omitempty"`
	Status            RoleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RoleListV4 contains a list of RoleV4
type RoleListV4 struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RoleV4 `json:"items"`
}

// Marshal serializes a spec into binary data.
func (spec *RoleSpecV4) Marshal() ([]byte, error) {
	return (*types.RoleSpecV4)(spec).Marshal()
}

// Unmarshal deserializes a spec from binary data.
func (spec *RoleSpecV4) Unmarshal(data []byte) error {
	return (*types.RoleSpecV4)(spec).Unmarshal(data)
}

// DeepCopyInto deep-copies one role spec into another.
// Required to satisfy runtime.Object interface.
func (spec *RoleSpecV4) DeepCopyInto(out *RoleSpecV4) {
	data, err := spec.Marshal()
	if err != nil {
		panic(err)
	}
	*out = RoleSpecV4{}
	if err = out.Unmarshal(data); err != nil {
		panic(err)
	}
}

// SetErrorStatus sets an error status of a role object.
func (role *RoleV4) SetErrorStatus(err error) {
	role.Status.ResourceStatus.SetLastError(err)
}

// ToTeleportRole converts a Kubernetes resource into a Teleport role.
func (role *RoleV4) ToTeleportRole() types.Role {
	return &types.RoleV4{
		Kind:     types.KindRole,
		Version:  types.V4,
		Metadata: types.Metadata{Name: role.Name},
		Spec:     types.RoleSpecV4(role.Spec),
	}
}

// Register role types

func init() {
	register(
		&RoleV4{},
		&RoleListV4{},
	)
}
