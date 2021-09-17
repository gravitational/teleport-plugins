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
	"fmt"
	"time"

	"github.com/gravitational/teleport/api/defaults"

	"github.com/gravitational/trace"
)

// ClusterNetworkingConfig defines cluster networking configuration. This is
// a configuration resource, never create more than one instance of it.
type ClusterNetworkingConfig interface {
	Resource

	// GetClientIdleTimeout returns client idle timeout setting
	GetClientIdleTimeout() time.Duration

	// SetClientIdleTimeout sets client idle timeout setting
	SetClientIdleTimeout(t time.Duration)

	// GetKeepAliveInterval gets the keep-alive interval for server to client
	// connections.
	GetKeepAliveInterval() time.Duration

	// SetKeepAliveInterval sets the keep-alive interval for server to client
	// connections.
	SetKeepAliveInterval(t time.Duration)

	// GetKeepAliveCountMax gets the number of missed keep-alive messages before
	// the server disconnects the client.
	GetKeepAliveCountMax() int64

	// SetKeepAliveCountMax sets the number of missed keep-alive messages before
	// the server disconnects the client.
	SetKeepAliveCountMax(c int64)

	// GetSessionControlTimeout gets the session control timeout.
	GetSessionControlTimeout() time.Duration

	// SetSessionControlTimeout sets the session control timeout.
	SetSessionControlTimeout(t time.Duration)
}

// NewClusterNetworkingConfig is a convenience method to create ClusterNetworkingConfigV2.
func NewClusterNetworkingConfig(spec ClusterNetworkingConfigSpecV2) (ClusterNetworkingConfig, error) {
	netConfig := ClusterNetworkingConfigV2{
		Kind:    KindClusterNetworkingConfig,
		Version: V2,
		Metadata: Metadata{
			Name:      MetaNameClusterNetworkingConfig,
			Namespace: defaults.Namespace,
		},
		Spec: spec,
	}

	if err := netConfig.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &netConfig, nil
}

// DefaultClusterNetworkingConfig returns the default cluster networking config.
func DefaultClusterNetworkingConfig() ClusterNetworkingConfig {
	config, _ := NewClusterNetworkingConfig(ClusterNetworkingConfigSpecV2{})
	return config
}

// GetVersion returns resource version.
func (c *ClusterNetworkingConfigV2) GetVersion() string {
	return c.Version
}

// GetName returns the name of the resource.
func (c *ClusterNetworkingConfigV2) GetName() string {
	return c.Metadata.Name
}

// SetName sets the name of the resource.
func (c *ClusterNetworkingConfigV2) SetName(name string) {
	c.Metadata.Name = name
}

// SetExpiry sets expiry time for the object.
func (c *ClusterNetworkingConfigV2) SetExpiry(expires time.Time) {
	c.Metadata.SetExpiry(expires)
}

// Expiry returns object expiry setting.
func (c *ClusterNetworkingConfigV2) Expiry() time.Time {
	return c.Metadata.Expiry()
}

// SetTTL sets Expires header using the provided clock.
// Use SetExpiry instead.
// DELETE IN 7.0.0
func (c *ClusterNetworkingConfigV2) SetTTL(clock Clock, ttl time.Duration) {
	c.Metadata.SetTTL(clock, ttl)
}

// GetMetadata returns object metadata.
func (c *ClusterNetworkingConfigV2) GetMetadata() Metadata {
	return c.Metadata
}

// GetResourceID returns resource ID.
func (c *ClusterNetworkingConfigV2) GetResourceID() int64 {
	return c.Metadata.ID
}

// SetResourceID sets resource ID.
func (c *ClusterNetworkingConfigV2) SetResourceID(id int64) {
	c.Metadata.ID = id
}

// GetKind returns resource kind.
func (c *ClusterNetworkingConfigV2) GetKind() string {
	return c.Kind
}

// GetSubKind returns resource subkind.
func (c *ClusterNetworkingConfigV2) GetSubKind() string {
	return c.SubKind
}

// SetSubKind sets resource subkind.
func (c *ClusterNetworkingConfigV2) SetSubKind(sk string) {
	c.SubKind = sk
}

// GetClientIdleTimeout returns client idle timeout setting.
func (c *ClusterNetworkingConfigV2) GetClientIdleTimeout() time.Duration {
	return c.Spec.ClientIdleTimeout.Duration()
}

// SetClientIdleTimeout sets client idle timeout setting.
func (c *ClusterNetworkingConfigV2) SetClientIdleTimeout(d time.Duration) {
	c.Spec.ClientIdleTimeout = Duration(d)
}

// GetKeepAliveInterval gets the keep-alive interval.
func (c *ClusterNetworkingConfigV2) GetKeepAliveInterval() time.Duration {
	return c.Spec.KeepAliveInterval.Duration()
}

// SetKeepAliveInterval sets the keep-alive interval.
func (c *ClusterNetworkingConfigV2) SetKeepAliveInterval(t time.Duration) {
	c.Spec.KeepAliveInterval = Duration(t)
}

// GetKeepAliveCountMax gets the number of missed keep-alive messages before
// the server disconnects the client.
func (c *ClusterNetworkingConfigV2) GetKeepAliveCountMax() int64 {
	return c.Spec.KeepAliveCountMax
}

// SetKeepAliveCountMax sets the number of missed keep-alive messages before
// the server disconnects the client.
func (c *ClusterNetworkingConfigV2) SetKeepAliveCountMax(m int64) {
	c.Spec.KeepAliveCountMax = m
}

// GetSessionControlTimeout gets the session control timeout.
func (c *ClusterNetworkingConfigV2) GetSessionControlTimeout() time.Duration {
	return c.Spec.SessionControlTimeout.Duration()
}

// SetSessionControlTimeout sets the session control timeout.
func (c *ClusterNetworkingConfigV2) SetSessionControlTimeout(d time.Duration) {
	c.Spec.SessionControlTimeout = Duration(d)
}

// CheckAndSetDefaults verifies the constraints for ClusterNetworkingConfig.
func (c *ClusterNetworkingConfigV2) CheckAndSetDefaults() error {
	// Make sure we have defaults for all metadata fields.
	err := c.Metadata.CheckAndSetDefaults()
	if err != nil {
		return trace.Wrap(err)
	}

	// Set the keep-alive interval and max missed keep-alives.
	if c.Spec.KeepAliveInterval.Duration() == 0 {
		c.Spec.KeepAliveInterval = NewDuration(defaults.KeepAliveInterval)
	}
	if c.Spec.KeepAliveCountMax == 0 {
		c.Spec.KeepAliveCountMax = int64(defaults.KeepAliveCountMax)
	}

	return nil
}

// String returns string representation of cluster networking configuration.
func (c *ClusterNetworkingConfigV2) String() string {
	return fmt.Sprintf("ClusterNetworkingConfig(ClientIdleTimeout=%v,KeepAliveInterval=%v,KeepAliveCountMax=%v,SessionControlTimeout=%v)",
		c.Spec.ClientIdleTimeout, c.Spec.KeepAliveInterval, c.Spec.KeepAliveCountMax, c.Spec.SessionControlTimeout)
}
