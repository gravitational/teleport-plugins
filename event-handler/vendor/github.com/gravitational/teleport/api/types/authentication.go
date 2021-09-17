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
	"bytes"
	"fmt"
	"time"

	"github.com/gravitational/teleport/api/constants"
	"github.com/gravitational/teleport/api/defaults"
	"github.com/gravitational/teleport/api/utils/tlsutils"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gravitational/trace"
)

// AuthPreference defines the authentication preferences for a specific
// cluster. It defines the type (local, oidc) and second factor (off, otp, oidc).
// AuthPreference is a configuration resource, never create more than one instance
// of it.
type AuthPreference interface {
	// Resource provides common resource properties.
	ResourceWithOrigin

	// GetType gets the type of authentication: local, saml, or oidc.
	GetType() string
	// SetType sets the type of authentication: local, saml, or oidc.
	SetType(string)

	// GetSecondFactor gets the type of second factor: off, otp or u2f.
	GetSecondFactor() constants.SecondFactorType
	// SetSecondFactor sets the type of second factor: off, otp, or u2f.
	SetSecondFactor(constants.SecondFactorType)

	// GetConnectorName gets the name of the OIDC or SAML connector to use. If
	// this value is empty, we fall back to the first connector in the backend.
	GetConnectorName() string
	// SetConnectorName sets the name of the OIDC or SAML connector to use. If
	// this value is empty, we fall back to the first connector in the backend.
	SetConnectorName(string)

	// GetU2F gets the U2F configuration settings.
	GetU2F() (*U2F, error)
	// SetU2F sets the U2F configuration settings.
	SetU2F(*U2F)

	// GetRequireSessionMFA returns true when all sessions in this cluster
	// require an MFA check.
	GetRequireSessionMFA() bool

	// String represents a human readable version of authentication settings.
	String() string
}

// NewAuthPreference is a convenience method to to create AuthPreferenceV2.
func NewAuthPreference(spec AuthPreferenceSpecV2) (AuthPreference, error) {
	return newAuthPreferenceWithLabels(spec, map[string]string{})
}

// NewAuthPreferenceFromConfigFile is a convenience method to create
// AuthPreferenceV2 labelled as originating from config file.
func NewAuthPreferenceFromConfigFile(spec AuthPreferenceSpecV2) (AuthPreference, error) {
	return newAuthPreferenceWithLabels(spec, map[string]string{
		OriginLabel: OriginConfigFile,
	})
}

// NewAuthPreferenceWithLabels is a convenience method to create
// AuthPreferenceV2 with a specific map of labels.
func newAuthPreferenceWithLabels(spec AuthPreferenceSpecV2, labels map[string]string) (AuthPreference, error) {
	pref := AuthPreferenceV2{
		Kind:    KindClusterAuthPreference,
		Version: V2,
		Metadata: Metadata{
			Name:      MetaNameClusterAuthPreference,
			Namespace: defaults.Namespace,
			Labels:    labels,
		},
		Spec: spec,
	}

	if err := pref.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &pref, nil
}

// DefaultAuthPreference returns the default authentication preferences.
func DefaultAuthPreference() AuthPreference {
	return &AuthPreferenceV2{
		Kind:    KindClusterAuthPreference,
		Version: V2,
		Metadata: Metadata{
			Name:      MetaNameClusterAuthPreference,
			Namespace: defaults.Namespace,
			Labels: map[string]string{
				OriginLabel: OriginDefaults,
			},
		},
		Spec: AuthPreferenceSpecV2{
			Type:         constants.Local,
			SecondFactor: constants.SecondFactorOTP,
		},
	}
}

// GetVersion returns resource version.
func (c *AuthPreferenceV2) GetVersion() string {
	return c.Version
}

// GetName returns the name of the resource.
func (c *AuthPreferenceV2) GetName() string {
	return c.Metadata.Name
}

// SetName sets the name of the resource.
func (c *AuthPreferenceV2) SetName(e string) {
	c.Metadata.Name = e
}

// SetExpiry sets expiry time for the object.
func (c *AuthPreferenceV2) SetExpiry(expires time.Time) {
	c.Metadata.SetExpiry(expires)
}

// Expiry returns object expiry setting.
func (c *AuthPreferenceV2) Expiry() time.Time {
	return c.Metadata.Expiry()
}

// SetTTL sets Expires header using the provided clock.
// Use SetExpiry instead.
// DELETE IN 7.0.0
func (c *AuthPreferenceV2) SetTTL(clock Clock, ttl time.Duration) {
	c.Metadata.SetTTL(clock, ttl)
}

// GetMetadata returns object metadata.
func (c *AuthPreferenceV2) GetMetadata() Metadata {
	return c.Metadata
}

// GetResourceID returns resource ID.
func (c *AuthPreferenceV2) GetResourceID() int64 {
	return c.Metadata.ID
}

// SetResourceID sets resource ID.
func (c *AuthPreferenceV2) SetResourceID(id int64) {
	c.Metadata.ID = id
}

// Origin returns the origin value of the resource.
func (c *AuthPreferenceV2) Origin() string {
	return c.Metadata.Origin()
}

// SetOrigin sets the origin value of the resource.
func (c *AuthPreferenceV2) SetOrigin(origin string) {
	c.Metadata.SetOrigin(origin)
}

// GetKind returns resource kind.
func (c *AuthPreferenceV2) GetKind() string {
	return c.Kind
}

// GetSubKind returns resource subkind.
func (c *AuthPreferenceV2) GetSubKind() string {
	return c.SubKind
}

// SetSubKind sets resource subkind.
func (c *AuthPreferenceV2) SetSubKind(sk string) {
	c.SubKind = sk
}

// GetType returns the type of authentication.
func (c *AuthPreferenceV2) GetType() string {
	return c.Spec.Type
}

// SetType sets the type of authentication.
func (c *AuthPreferenceV2) SetType(s string) {
	c.Spec.Type = s
}

// GetSecondFactor returns the type of second factor.
func (c *AuthPreferenceV2) GetSecondFactor() constants.SecondFactorType {
	return c.Spec.SecondFactor
}

// SetSecondFactor sets the type of second factor.
func (c *AuthPreferenceV2) SetSecondFactor(s constants.SecondFactorType) {
	c.Spec.SecondFactor = s
}

// GetConnectorName gets the name of the OIDC or SAML connector to use. If
// this value is empty, we fall back to the first connector in the backend.
func (c *AuthPreferenceV2) GetConnectorName() string {
	return c.Spec.ConnectorName
}

// SetConnectorName sets the name of the OIDC or SAML connector to use. If
// this value is empty, we fall back to the first connector in the backend.
func (c *AuthPreferenceV2) SetConnectorName(cn string) {
	c.Spec.ConnectorName = cn
}

// GetU2F gets the U2F configuration settings.
func (c *AuthPreferenceV2) GetU2F() (*U2F, error) {
	if c.Spec.U2F == nil {
		return nil, trace.NotFound("U2F is not configured in this cluster, please contact your administrator and ask them to follow https://goteleport.com/docs/access-controls/guides/u2f/")
	}
	return c.Spec.U2F, nil
}

// SetU2F sets the U2F configuration settings.
func (c *AuthPreferenceV2) SetU2F(u2f *U2F) {
	c.Spec.U2F = u2f
}

// GetRequireSessionMFA returns true when all sessions in this cluster require
// an MFA check.
func (c *AuthPreferenceV2) GetRequireSessionMFA() bool {
	return c.Spec.RequireSessionMFA
}

// CheckAndSetDefaults verifies the constraints for AuthPreference.
func (c *AuthPreferenceV2) CheckAndSetDefaults() error {
	// make sure we have defaults for all metadata fields
	err := c.Metadata.CheckAndSetDefaults()
	if err != nil {
		return trace.Wrap(err)
	}

	// Make sure origin value is always set.
	if c.Origin() == "" {
		c.SetOrigin(OriginDynamic)
	}

	// if nothing is passed in, set defaults
	if c.Spec.Type == "" {
		c.Spec.Type = constants.Local
	}
	if c.Spec.SecondFactor == "" {
		c.Spec.SecondFactor = constants.SecondFactorOTP
	}

	// make sure type makes sense
	switch c.Spec.Type {
	case constants.Local, constants.OIDC, constants.SAML, constants.Github:
	default:
		return trace.BadParameter("authentication type %q not supported", c.Spec.Type)
	}

	// make sure second factor makes sense
	switch c.Spec.SecondFactor {
	case constants.SecondFactorOff, constants.SecondFactorOTP:
	case constants.SecondFactorU2F, constants.SecondFactorOn, constants.SecondFactorOptional:
		if c.Spec.U2F == nil {
			return trace.BadParameter("missing required U2F configuration for second factor type %q", c.Spec.SecondFactor)
		}
		if err := c.Spec.U2F.Check(); err != nil {
			return trace.Wrap(err)
		}
	default:
		return trace.BadParameter("second factor type %q not supported", c.Spec.SecondFactor)
	}

	return nil
}

// String represents a human readable version of authentication settings.
func (c *AuthPreferenceV2) String() string {
	return fmt.Sprintf("AuthPreference(Type=%q,SecondFactor=%q)", c.Spec.Type, c.Spec.SecondFactor)
}

func (u *U2F) Check() error {
	if u.AppID == "" {
		return trace.BadParameter("u2f configuration missing app_id")
	}
	if len(u.Facets) == 0 {
		return trace.BadParameter("u2f configuration missing facets")
	}
	for _, ca := range u.DeviceAttestationCAs {
		if _, err := tlsutils.ParseCertificatePEM([]byte(ca)); err != nil {
			return trace.BadParameter("u2f configuration has an invalid attestation CA: %v", err)
		}
	}
	return nil
}

// NewMFADevice creates a new MFADevice with the given name. Caller must set
// the Device field in the returned MFADevice.
func NewMFADevice(name, id string, addedAt time.Time) *MFADevice {
	return &MFADevice{
		Kind: KindMFADevice,
		Metadata: Metadata{
			Name: name,
		},
		Id:       id,
		AddedAt:  addedAt,
		LastUsed: addedAt,
	}
}

// CheckAndSetDefaults validates MFADevice fields and populates empty fields
// with default values.
func (d *MFADevice) CheckAndSetDefaults() error {
	if err := d.Metadata.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	if d.Kind == "" {
		return trace.BadParameter("MFADevice missing Kind field")
	}
	if d.Version == "" {
		d.Version = V1
	}
	if d.Id == "" {
		return trace.BadParameter("MFADevice missing ID field")
	}
	if d.AddedAt.IsZero() {
		return trace.BadParameter("MFADevice missing AddedAt field")
	}
	if d.LastUsed.IsZero() {
		return trace.BadParameter("MFADevice missing LastUsed field")
	}
	if d.LastUsed.Before(d.AddedAt) {
		return trace.BadParameter("MFADevice LastUsed field must be earlier than AddedAt")
	}
	if d.Device == nil {
		return trace.BadParameter("MFADevice missing Device field")
	}
	return nil
}

func (d *MFADevice) GetKind() string                       { return d.Kind }
func (d *MFADevice) GetSubKind() string                    { return d.SubKind }
func (d *MFADevice) SetSubKind(sk string)                  { d.SubKind = sk }
func (d *MFADevice) GetVersion() string                    { return d.Version }
func (d *MFADevice) GetMetadata() Metadata                 { return d.Metadata }
func (d *MFADevice) GetName() string                       { return d.Metadata.GetName() }
func (d *MFADevice) SetName(n string)                      { d.Metadata.SetName(n) }
func (d *MFADevice) GetResourceID() int64                  { return d.Metadata.ID }
func (d *MFADevice) SetResourceID(id int64)                { d.Metadata.SetID(id) }
func (d *MFADevice) Expiry() time.Time                     { return d.Metadata.Expiry() }
func (d *MFADevice) SetExpiry(exp time.Time)               { d.Metadata.SetExpiry(exp) }
func (d *MFADevice) SetTTL(clock Clock, ttl time.Duration) { d.Metadata.SetTTL(clock, ttl) }

// MFAType returns the human-readable name of the MFA protocol of this device.
func (d *MFADevice) MFAType() string {
	switch d.Device.(type) {
	case *MFADevice_Totp:
		return "TOTP"
	case *MFADevice_U2F:
		return "U2F"
	default:
		return "unknown"
	}
}

func (d *MFADevice) MarshalJSON() ([]byte, error) {
	buf := new(bytes.Buffer)
	err := (&jsonpb.Marshaler{}).Marshal(buf, d)
	return buf.Bytes(), trace.Wrap(err)
}

func (d *MFADevice) UnmarshalJSON(buf []byte) error {
	return jsonpb.Unmarshal(bytes.NewReader(buf), d)
}
