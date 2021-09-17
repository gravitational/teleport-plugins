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

package types

import (
	"time"

	"github.com/gravitational/trace"
)

// NetworkRestrictions defines network restrictions applied to SSH session.
type NetworkRestrictions interface {
	Resource
	// GetAllow returns a list of allowed network addresses
	GetAllow() []AddressCondition
	// SetAllow sets a list of allowed network addresses
	SetAllow(allow []AddressCondition)
	// GetDeny returns a list of denied network addresses (overrides Allow list)
	GetDeny() []AddressCondition
	// SetDeny sets a list of denied network addresses (overrides Allow list)
	SetDeny(deny []AddressCondition)
}

// NewNetworkRestrictions creates a new NetworkRestrictions with the given name.
func NewNetworkRestrictions() NetworkRestrictions {
	return &NetworkRestrictionsV4{
		Kind:    KindNetworkRestrictions,
		Version: V4,
		Metadata: Metadata{
			Name: MetaNameNetworkRestrictions,
		},
	}
}

// CheckAndSetDefaults validates NetworkRestrictions fields and populates empty fields
// with default values.
func (r *NetworkRestrictionsV4) CheckAndSetDefaults() error {
	r.Metadata.Name = MetaNameNetworkRestrictions

	if err := r.Metadata.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	if r.Kind == "" {
		return trace.BadParameter("NetworkRestrictions missing Kind field")
	}
	if r.Version == "" {
		r.Version = V1
	}
	return nil
}

func (r *NetworkRestrictionsV4) GetKind() string {
	return r.Kind
}

func (r *NetworkRestrictionsV4) GetSubKind() string {
	return r.SubKind
}

func (r *NetworkRestrictionsV4) SetSubKind(sk string) {
	r.SubKind = sk
}

func (r *NetworkRestrictionsV4) GetVersion() string {
	return r.Version
}

func (r *NetworkRestrictionsV4) GetMetadata() Metadata {
	return r.Metadata
}

func (r *NetworkRestrictionsV4) GetName() string {
	return r.Metadata.GetName()
}

func (r *NetworkRestrictionsV4) SetName(n string) {
	r.Metadata.SetName(n)
}

func (r *NetworkRestrictionsV4) GetResourceID() int64 {
	return r.Metadata.ID
}

func (r *NetworkRestrictionsV4) SetResourceID(id int64) {
	r.Metadata.SetID(id)
}

func (r *NetworkRestrictionsV4) Expiry() time.Time {
	return r.Metadata.Expiry()
}

func (r *NetworkRestrictionsV4) SetExpiry(exp time.Time) {
	r.Metadata.SetExpiry(exp)
}

func (r *NetworkRestrictionsV4) GetAllow() []AddressCondition {
	return r.Spec.Allow
}

func (r *NetworkRestrictionsV4) SetAllow(allow []AddressCondition) {
	r.Spec.Allow = allow
}

func (r *NetworkRestrictionsV4) GetDeny() []AddressCondition {
	return r.Spec.Deny
}

func (r *NetworkRestrictionsV4) SetDeny(deny []AddressCondition) {
	r.Spec.Deny = deny
}
