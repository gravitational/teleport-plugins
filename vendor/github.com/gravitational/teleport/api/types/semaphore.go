/*
Copyright 2020 Gravitational, Inc.

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
	"context"
	"fmt"
	"time"

	"github.com/gravitational/teleport/api/constants"
	"github.com/gravitational/teleport/api/defaults"

	"github.com/gravitational/trace"
)

// SemaphoreKindConnection is the semaphore kind used by
// the Concurrent Session Control feature to limit concurrent
// connections (corresponds to the `max_connections`
// role option).
const SemaphoreKindConnection = "connection"

// Semaphore represents distributed semaphore concept
type Semaphore interface {
	// Resource contains common resource values
	Resource
	// CheckAndSetDefaults checks and sets default parameters
	CheckAndSetDefaults() error
	// Contains checks if lease is member of this semaphore.
	Contains(lease SemaphoreLease) bool
	// Acquire attempts to acquire a lease with this semaphore.
	Acquire(leaseID string, params AcquireSemaphoreRequest) (*SemaphoreLease, error)
	// KeepAlive attempts to update the expiry of an existent lease.
	KeepAlive(lease SemaphoreLease) error
	// Cancel attempts to cancel an existent lease.
	Cancel(lease SemaphoreLease) error
	// LeaseRefs grants access to the underlying list
	// of lease references.
	LeaseRefs() []SemaphoreLeaseRef
	// RemoveExpiredLeases removes expired leases
	RemoveExpiredLeases(now time.Time)
}

// ConfigureSemaphore configures an empty semaphore resource matching
// these acquire parameters.
func (s *AcquireSemaphoreRequest) ConfigureSemaphore() (Semaphore, error) {
	sem := SemaphoreV3{
		Kind:    KindSemaphore,
		SubKind: s.SemaphoreKind,
		Version: V3,
		Metadata: Metadata{
			Name:      s.SemaphoreName,
			Namespace: defaults.Namespace,
		},
	}
	sem.SetExpiry(s.Expires)
	if err := sem.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &sem, nil
}

// Check verifies that all required parameters have been supplied.
func (s *AcquireSemaphoreRequest) Check() error {
	if s.SemaphoreKind == "" {
		return trace.BadParameter("missing parameter SemaphoreKind")
	}
	if s.SemaphoreName == "" {
		return trace.BadParameter("missing parameter SemaphoreName")
	}
	if s.MaxLeases == 0 {
		return trace.BadParameter("missing parameter MaxLeases")
	}
	if s.Expires.IsZero() {
		return trace.BadParameter("missing parameter Expires")
	}
	return nil
}

// CheckAndSetDefaults checks and sets default values
func (l *SemaphoreLease) CheckAndSetDefaults() error {
	if l.SemaphoreKind == "" {
		return trace.BadParameter("missing parameter SemaphoreKind")
	}
	if l.SemaphoreName == "" {
		return trace.BadParameter("missing parameter SemaphoreName")
	}
	if l.LeaseID == "" {
		return trace.BadParameter("missing parameter LeaseID")
	}
	if l.Expires.IsZero() {
		return trace.BadParameter("missing lease expiry time")
	}
	return nil
}

// Contains checks if lease is member of this semaphore.
func (c *SemaphoreV3) Contains(lease SemaphoreLease) bool {
	if lease.SemaphoreKind != c.GetSubKind() || lease.SemaphoreName != c.GetName() {
		return false
	}
	for _, ref := range c.Spec.Leases {
		if ref.LeaseID == lease.LeaseID {
			return true
		}
	}
	return false
}

// Acquire attempts to acquire a lease with this semaphore.
func (c *SemaphoreV3) Acquire(leaseID string, params AcquireSemaphoreRequest) (*SemaphoreLease, error) {
	if params.SemaphoreKind != c.GetSubKind() || params.SemaphoreName != c.GetName() {
		return nil, trace.BadParameter("cannot acquire, params do not match")
	}

	if c.leaseCount() >= params.MaxLeases {
		return nil, trace.LimitExceeded("cannot acquire semaphore %s/%s (%s)",
			c.GetSubKind(),
			c.GetName(),
			constants.MaxLeases,
		)
	}

	for _, ref := range c.Spec.Leases {
		if ref.LeaseID == leaseID {
			return nil, trace.AlreadyExists("semaphore lease already exists: %q", leaseID)
		}
	}

	if params.Expires.After(c.Expiry()) {
		c.SetExpiry(params.Expires)
	}

	c.Spec.Leases = append(c.Spec.Leases, SemaphoreLeaseRef{
		LeaseID: leaseID,
		Expires: params.Expires,
		Holder:  params.Holder,
	})

	return &SemaphoreLease{
		SemaphoreKind: params.SemaphoreKind,
		SemaphoreName: params.SemaphoreName,
		LeaseID:       leaseID,
		Expires:       params.Expires,
	}, nil
}

// KeepAlive attempts to update the expiry of an existent lease.
func (c *SemaphoreV3) KeepAlive(lease SemaphoreLease) error {
	if lease.SemaphoreKind != c.GetSubKind() || lease.SemaphoreName != c.GetName() {
		return trace.BadParameter("cannot keepalive, lease does not match")
	}
	for i := range c.Spec.Leases {
		if c.Spec.Leases[i].LeaseID == lease.LeaseID {
			c.Spec.Leases[i].Expires = lease.Expires
			if lease.Expires.After(c.Expiry()) {
				c.SetExpiry(lease.Expires)
			}
			return nil
		}
	}
	return trace.NotFound("cannot keepalive, lease not found: %q", lease.LeaseID)
}

// Cancel attempts to cancel an existent lease.
func (c *SemaphoreV3) Cancel(lease SemaphoreLease) error {
	if lease.SemaphoreKind != c.GetSubKind() || lease.SemaphoreName != c.GetName() {
		return trace.BadParameter("cannot cancel, lease does not match")
	}
	for i, ref := range c.Spec.Leases {
		if ref.LeaseID == lease.LeaseID {
			c.Spec.Leases = append(c.Spec.Leases[:i], c.Spec.Leases[i+1:]...)
			return nil
		}
	}
	return trace.NotFound("cannot cancel, lease not found: %q", lease.LeaseID)
}

// RemoveExpiredLeases removes expired leases
func (c *SemaphoreV3) RemoveExpiredLeases(now time.Time) {
	// See https://github.com/golang/go/wiki/SliceTricks#filtering-without-allocating
	filtered := c.Spec.Leases[:0]
	for _, lease := range c.Spec.Leases {
		if lease.Expires.After(now) {
			filtered = append(filtered, lease)
		}
	}
	c.Spec.Leases = filtered
}

// leaseCount returns the number of active leases
func (c *SemaphoreV3) leaseCount() int64 {
	return int64(len(c.Spec.Leases))
}

// LeaseRefs grants access to the underlying list
// of lease references
func (c *SemaphoreV3) LeaseRefs() []SemaphoreLeaseRef {
	return c.Spec.Leases
}

// GetVersion returns resource version
func (c *SemaphoreV3) GetVersion() string {
	return c.Version
}

// GetSubKind returns resource subkind
func (c *SemaphoreV3) GetSubKind() string {
	return c.SubKind
}

// SetSubKind sets resource subkind
func (c *SemaphoreV3) SetSubKind(sk string) {
	c.SubKind = sk
}

// GetKind returns resource kind
func (c *SemaphoreV3) GetKind() string {
	return c.Kind
}

// GetResourceID returns resource ID
func (c *SemaphoreV3) GetResourceID() int64 {
	return c.Metadata.ID
}

// SetResourceID sets resource ID
func (c *SemaphoreV3) SetResourceID(id int64) {
	c.Metadata.ID = id
}

// GetName returns the name of the cluster.
func (c *SemaphoreV3) GetName() string {
	return c.Metadata.Name
}

// SetName sets the name of the cluster.
func (c *SemaphoreV3) SetName(e string) {
	c.Metadata.Name = e
}

// Expiry returns object expiry setting
func (c *SemaphoreV3) Expiry() time.Time {
	return c.Metadata.Expiry()
}

// SetExpiry sets expiry time for the object
func (c *SemaphoreV3) SetExpiry(expires time.Time) {
	c.Metadata.SetExpiry(expires)
}

// SetTTL sets Expires header using the provided clock.
// Use SetExpiry instead.
// DELETE IN 7.0.0
func (c *SemaphoreV3) SetTTL(clock Clock, ttl time.Duration) {
	c.Metadata.SetTTL(clock, ttl)
}

// GetMetadata returns object metadata
func (c *SemaphoreV3) GetMetadata() Metadata {
	return c.Metadata
}

// String represents a human readable version of the semaphore.
func (c *SemaphoreV3) String() string {
	return fmt.Sprintf("Semaphore(kind=%v, name=%v, leases=%v)",
		c.SubKind, c.Metadata.Name, c.leaseCount())
}

// CheckAndSetDefaults checks validity of all parameters and sets defaults.
func (c *SemaphoreV3) CheckAndSetDefaults() error {
	// make sure we have defaults for all metadata fields
	err := c.Metadata.CheckAndSetDefaults()
	if err != nil {
		return trace.Wrap(err)
	}
	// While theoretically there are scenarios with non-expiring semaphores
	// however the flow don't need them right now, and they add a lot of edge
	// cases, so the code does not support them.
	if c.Expiry().IsZero() {
		return trace.BadParameter("set semaphore expiry time")
	}
	if c.SubKind == "" {
		return trace.BadParameter("supply semaphore SubKind parameter")
	}
	return nil
}

// Semaphores provides ability to control
// how many shared resources of some kind are acquired at the same time,
// used to implement concurrent sessions control in a distributed environment
type Semaphores interface {
	// AcquireSemaphore acquires lease with requested resources from semaphore
	AcquireSemaphore(ctx context.Context, params AcquireSemaphoreRequest) (*SemaphoreLease, error)
	// KeepAliveSemaphoreLease updates semaphore lease
	KeepAliveSemaphoreLease(ctx context.Context, lease SemaphoreLease) error
	// CancelSemaphoreLease cancels semaphore lease early
	CancelSemaphoreLease(ctx context.Context, lease SemaphoreLease) error
	// GetSemaphores returns a list of semaphores matching supplied filter.
	GetSemaphores(ctx context.Context, filter SemaphoreFilter) ([]Semaphore, error)
	// DeleteSemaphore deletes a semaphore matching supplied filter.
	DeleteSemaphore(ctx context.Context, filter SemaphoreFilter) error
}

// Match checks if the supplied semaphore matches this filter.
func (f *SemaphoreFilter) Match(sem Semaphore) bool {
	if f.SemaphoreKind != "" && f.SemaphoreKind != sem.GetSubKind() {
		return false
	}
	if f.SemaphoreName != "" && f.SemaphoreName != sem.GetName() {
		return false
	}
	return true
}
