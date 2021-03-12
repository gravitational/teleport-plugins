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

// Package types contains all types and logic required by the Teleport API.
package types

import (
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/gravitational/teleport/api/utils"

	"github.com/gravitational/trace"

	"github.com/vulcand/predicate"
)

// AccessRequest is a request for temporarily granted roles
type AccessRequest interface {
	Resource
	// GetUser gets the name of the requesting user
	GetUser() string
	// GetRoles gets the roles being requested by the user
	GetRoles() []string
	// SetRoles overrides the roles being requested by the user
	SetRoles([]string)
	// GetState gets the current state of the request
	GetState() RequestState
	// SetState sets the approval state of the request
	SetState(RequestState) error
	// GetCreationTime gets the time at which the request was
	// originally registered with the auth server.
	GetCreationTime() time.Time
	// SetCreationTime sets the creation time of the request.
	SetCreationTime(time.Time)
	// GetAccessExpiry gets the upper limit for which this request
	// may be considered active.
	GetAccessExpiry() time.Time
	// SetAccessExpiry sets the upper limit for which this request
	// may be considered active.
	SetAccessExpiry(time.Time)
	// GetRequestReason gets the reason for the request's creation.
	GetRequestReason() string
	// SetRequestReason sets the reason for the request's creation.
	SetRequestReason(string)
	// GetResolveReason gets the reason for the request's resolution.
	GetResolveReason() string
	// SetResolveReason sets the reason for the request's resolution.
	SetResolveReason(string)
	// GetResolveAnnotations gets the annotations associated with
	// the request's resolution.
	GetResolveAnnotations() map[string][]string
	// SetResolveAnnotations sets the annotations associated with
	// the request's resolution.
	SetResolveAnnotations(map[string][]string)
	// GetSystemAnnotations gets the teleport-applied annotations.
	GetSystemAnnotations() map[string][]string
	// SetSystemAnnotations sets the teleport-applied annotations.
	SetSystemAnnotations(map[string][]string)
	// GetOriginalRoles gets the original (pre-override) role list.
	GetOriginalRoles() []string
	// SetThresholds sets the review thresholds (internal use only).
	SetThresholds([]AccessReviewThreshold)
	// SetRoleThresholdMapping sets the rtm (internal use only).
	SetRoleThresholdMapping(map[string]ThresholdIndexSets)
	// ApplyReview applies an access review.  The supplied parser must be pre-configured to evaluate
	// boolian expressions based on the review and its author.
	ApplyReview(AccessReview, predicate.Parser) error
	// GetReviews gets the list of currently applied access reviews.
	GetReviews() []AccessReview
	// GetSuggestedReviewers gets the suggested reviewer list.
	GetSuggestedReviewers() []string
	// SetSuggestedReviewers sets the suggested reviewer list.
	SetSuggestedReviewers([]string)
	// CheckAndSetDefaults validates the access request and
	// supplies default values where appropriate.
	CheckAndSetDefaults() error
	// Equals checks equality between access request values.
	Equals(AccessRequest) bool
}

// NewAccessRequest assembled an AccessRequest resource.
func NewAccessRequest(name string, user string, roles ...string) (AccessRequest, error) {
	req := AccessRequestV3{
		Kind:    KindAccessRequest,
		Version: V3,
		Metadata: Metadata{
			Name: name,
		},
		Spec: AccessRequestSpecV3{
			User:  user,
			Roles: roles,
			State: RequestState_PENDING,
		},
	}
	if err := req.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &req, nil
}

// GetUser gets User
func (r *AccessRequestV3) GetUser() string {
	return r.Spec.User
}

// GetRoles gets Roles
func (r *AccessRequestV3) GetRoles() []string {
	return r.Spec.Roles
}

// SetRoles sets Roles
func (r *AccessRequestV3) SetRoles(roles []string) {
	r.Spec.Roles = roles
}

// GetState gets State
func (r *AccessRequestV3) GetState() RequestState {
	return r.Spec.State
}

// SetState sets State
func (r *AccessRequestV3) SetState(state RequestState) error {
	if r.Spec.State.IsDenied() {
		if state.IsDenied() {
			return nil
		}
		return trace.BadParameter("cannot set request-state %q (already denied)", state.String())
	}
	r.Spec.State = state
	return nil
}

// GetCreationTime gets CreationTime
func (r *AccessRequestV3) GetCreationTime() time.Time {
	return r.Spec.Created
}

// SetCreationTime sets CreationTime
func (r *AccessRequestV3) SetCreationTime(t time.Time) {
	r.Spec.Created = t
}

// GetAccessExpiry gets AccessExpiry
func (r *AccessRequestV3) GetAccessExpiry() time.Time {
	return r.Spec.Expires
}

// SetAccessExpiry sets AccessExpiry
func (r *AccessRequestV3) SetAccessExpiry(expiry time.Time) {
	r.Spec.Expires = expiry
}

// GetRequestReason gets RequestReason
func (r *AccessRequestV3) GetRequestReason() string {
	return r.Spec.RequestReason
}

// SetRequestReason sets RequestReason
func (r *AccessRequestV3) SetRequestReason(reason string) {
	r.Spec.RequestReason = reason
}

// GetResolveReason gets ResolveReason
func (r *AccessRequestV3) GetResolveReason() string {
	return r.Spec.ResolveReason
}

// SetResolveReason sets ResolveReason
func (r *AccessRequestV3) SetResolveReason(reason string) {
	r.Spec.ResolveReason = reason
}

// GetResolveAnnotations gets ResolveAnnotations
func (r *AccessRequestV3) GetResolveAnnotations() map[string][]string {
	return r.Spec.ResolveAnnotations
}

// SetResolveAnnotations sets ResolveAnnotations
func (r *AccessRequestV3) SetResolveAnnotations(annotations map[string][]string) {
	r.Spec.ResolveAnnotations = annotations
}

// GetSystemAnnotations gets SystemAnnotations
func (r *AccessRequestV3) GetSystemAnnotations() map[string][]string {
	return r.Spec.SystemAnnotations
}

// SetSystemAnnotations sets SystemAnnotations
func (r *AccessRequestV3) SetSystemAnnotations(annotations map[string][]string) {
	r.Spec.SystemAnnotations = annotations
}

func (r *AccessRequestV3) GetOriginalRoles() []string {
	if l := len(r.Spec.RoleThresholdMapping); l == 0 || l == len(r.Spec.Roles) {
		// rtm is unspecified or original role list is unmodified
		return r.Spec.Roles
	}

	// role subselection has been applied.  calculate original roles
	// by collecting the keys of the rtm.
	roles := make([]string, 0, len(r.Spec.RoleThresholdMapping))
	for role := range r.Spec.RoleThresholdMapping {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}

// getThreshold is a helper for getting a threshold by its stored index.
func (r *AccessRequestV3) getThreshold(tid uint32) (AccessReviewThreshold, error) {
	idx := int(tid)
	if len(r.Spec.Thresholds) <= idx {
		return AccessReviewThreshold{}, trace.Errorf("threshold index '%d' out of range (this is a bug)", tid)
	}
	return r.Spec.Thresholds[idx], nil
}

// SetThresholds sets the review thresholds.
func (r *AccessRequestV3) SetThresholds(thresholds []AccessReviewThreshold) {
	r.Spec.Thresholds = thresholds
}

// SetRoleThresholdMapping sets the rtm (internal use only).
func (r *AccessRequestV3) SetRoleThresholdMapping(rtm map[string]ThresholdIndexSets) {
	r.Spec.RoleThresholdMapping = rtm
}

// ApplyReview applies an access review.  The supplied parser must be pre-configured to evaluate
// boolian expressions based on the review and its author.  Prefer using services.ApplyAccessReview
// rather than calling this method directly.
func (r *AccessRequestV3) ApplyReview(rev AccessReview, parser predicate.Parser) error {
	if !rev.State.IsApproved() && !rev.State.IsDenied() {
		return trace.BadParameter("invalid state proposal: %s (expected approval/denial)", rev.State)
	}

	// the default theshold should exist. if it does not, the request either is not fully
	// initialized (i.e. variable expansion has not been run yet) or the was inserted into
	// the backend by a teleport instance which does not support the review feature.
	if len(r.Spec.Thresholds) == 0 {
		return trace.BadParameter("request is uninitialized or does not support reviews")
	}

	for _, existingReview := range r.Spec.Reviews {
		if existingReview.Author == rev.Author {
			return trace.AccessDenied("user %q has already reviewed this request", rev.Author)
		}
	}

	// dedupe and sort roles to simplify comparing role lists
	rev.Roles = utils.Deduplicate(rev.Roles)
	sort.Strings(rev.Roles)

	// TODO(fspmarshall): Remove this restriction once role overrides
	// in reviews are fully supported.
	if len(rev.Roles) != 0 && len(rev.Roles) != len(r.Spec.RoleThresholdMapping) {
		return trace.NotImplemented("reviews cannot perform role subselection")
	}

	// matches collects the results of checking all relevant thresholds
	// to see if their filters match this review.
	matches := make(map[uint32]bool)
	var matchCount int

	roles := rev.Roles
	if len(roles) == 0 {
		roles = r.GetOriginalRoles()
	}

	// check for matches with all thresholds related to the roles selected by this review
	// and aggregate the results.
	for _, role := range roles {
		thresholdSets, ok := r.Spec.RoleThresholdMapping[role]
		if !ok {
			return trace.BadParameter("role %q is not a member of this request", role)
		}

		for _, tset := range thresholdSets.Sets {
			for _, tid := range tset.Indexes {
				if _, ok := matches[tid]; ok {
					// already checked this threshold
					continue
				}

				thresh, err := r.getThreshold(tid)
				if err != nil {
					return trace.Wrap(err)
				}

				match, err := thresh.MatchesFilter(parser)
				if err != nil {
					return trace.Wrap(err)
				}

				matches[tid] = match
				if match {
					matchCount++
				}
			}
		}
	}

	// record a list of all thresholds whose filters match this review.
	rev.ThresholdIndexes = make([]uint32, 0, matchCount)
	for tid, matched := range matches {
		if matched {
			rev.ThresholdIndexes = append(rev.ThresholdIndexes, tid)
		}
	}

	r.Spec.Reviews = append(r.Spec.Reviews, rev)

	// recalculate updated request state.
	return r.onReview()
}

// onReview checks if we've hit sufficient thresholds for a state-transition,
// and applies it if that is the case.
func (r *AccessRequestV3) onReview() error {
	if !r.Spec.State.IsPending() {
		// no further state-transitions are performed after we exit
		// the PENDING state.
		return nil
	}

	// TODO(fspmarshall): Rework this function to support role subselection.

	// approved keeps track of roles that have hit at least one
	// of their approval thresholds.
	approved := make(map[string]bool)
	// counts keeps track of the approval and denial counts for all thresholds.
	counts := make([]struct{ approval, denial uint32 }, len(r.Spec.Thresholds))

	// lastReview stores the most recently processed review.  Because processing
	// halts once approval is reached, this represents the Nth review where N is
	// the highest applicable approval threshold.
	var lastReview AccessReview

	// Iterate through all reviews and aggregate them against `counts`.
ProcessReviews:
	for _, rev := range r.Spec.Reviews {
		lastReview = rev
		for _, tid := range rev.ThresholdIndexes {
			idx := int(tid)
			if len(r.Spec.Thresholds) <= idx {
				return trace.Errorf("threshold index '%d' out of range (this is a bug)", idx)
			}
			if rev.State.IsApproved() {
				counts[idx].approval++
			}
			if rev.State.IsDenied() {
				counts[idx].denial++
			}
		}

		// If we hit any denial thresholds, short-circuit immediately
		for i, t := range r.Spec.Thresholds {

			if counts[i].denial >= t.Deny && t.Deny != 0 {
				// A single denial threshold has been met, deny entire request
				r.Spec.State = RequestState_DENIED
				r.Spec.ResolveReason = lastReview.Reason
				r.Spec.ResolveAnnotations = lastReview.Annotations
				return nil
			}
		}

		// check for roles that can be transitioned to an approved state
	CheckRoleApprovals:
		for role, thresholdSets := range r.Spec.RoleThresholdMapping {
			if approved[role] {
				// role was marked approved during a previous iteration
				continue CheckRoleApprovals
			}

			// iterate through all theshold sets.  All sets must have at least
			// one threshold which has hit its approval count in order for the
			// role to be considered approved.
		CheckThresholdSets:
			for _, thresholds := range thresholdSets.Sets {

				for _, tid := range thresholds.Indexes {
					idx := int(tid)
					if len(r.Spec.Thresholds) <= idx {
						return trace.Errorf("threshold index out of range %s/%d (this is a bug)", role, tid)
					}
					t := r.Spec.Thresholds[idx]

					if counts[idx].approval >= t.Approve && t.Approve != 0 {
						// this set contains a threshold which has met its approval condition.
						// skip to the next set.
						continue CheckThresholdSets
					}
				}

				// no thresholds met for this set. there may be additional roles/thresholds
				// which did meet their requirements this iteration, but there is no point
				// processing them unless this set has also hit its requirements.  we therefore
				// move immediately to processing the next review.
				continue ProcessReviews
			}

			// since we skip to the next review as soon as we see a set which has not hit any of its
			// approval scenarios, we know that if we get to this point the role must be approved.
			approved[role] = true
		}
		// If we got here, then we iterated across all roles in the rtm without hitting any that
		// had not met their approval scenario.  The request has hit an approved state and further
		// reviews will not be processed.
		break ProcessReviews
	}

	if len(approved) != len(r.Spec.RoleThresholdMapping) {
		// at least one role has not hit its approval threshold
		return nil
	}

	r.Spec.State = RequestState_APPROVED

	// resolve reasons and annotations are set only by the final review.
	r.Spec.ResolveReason = lastReview.Reason
	r.Spec.ResolveAnnotations = lastReview.Annotations
	r.SetExpiry(r.GetAccessExpiry())

	return nil
}

// GetReviews gets the list of currently applied access reviews.
func (r *AccessRequestV3) GetReviews() []AccessReview {
	return r.Spec.Reviews
}

// GetSuggestedReviewers gets the suggested reviewer list.
func (r *AccessRequestV3) GetSuggestedReviewers() []string {
	return r.Spec.SuggestedReviewers
}

// SetSuggestedReviewers sets the suggested reviewer list.
func (r *AccessRequestV3) SetSuggestedReviewers(reviewers []string) {
	r.Spec.SuggestedReviewers = reviewers
}

// CheckAndSetDefaults validates set values and sets default values
func (r *AccessRequestV3) CheckAndSetDefaults() error {
	if err := r.Metadata.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	if r.GetState().IsNone() {
		if err := r.SetState(RequestState_PENDING); err != nil {
			return trace.Wrap(err)
		}
	}

	// dedupe and sort roles to simplify comparing role lists
	r.Spec.Roles = utils.Deduplicate(r.Spec.Roles)
	sort.Strings(r.Spec.Roles)

	if err := r.Check(); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// Check validates AccessRequest values
func (r *AccessRequestV3) Check() error {
	if r.Kind == "" {
		return trace.BadParameter("access request kind not set")
	}
	if r.Version == "" {
		return trace.BadParameter("access request version not set")
	}
	if r.GetName() == "" {
		return trace.BadParameter("access request id not set")
	}
	if r.GetUser() == "" {
		return trace.BadParameter("access request user name not set")
	}
	if len(r.GetRoles()) < 1 {
		return trace.BadParameter("access request does not specify any roles")
	}
	if r.GetState().IsPending() {
		if r.GetResolveReason() != "" {
			return trace.BadParameter("pending requests cannot include resolve reason")
		}
		if len(r.GetResolveAnnotations()) != 0 {
			return trace.BadParameter("pending requests cannot include resolve annotations")
		}
	}
	return nil
}

// GetKind gets Kind
func (r *AccessRequestV3) GetKind() string {
	return r.Kind
}

// GetSubKind gets SubKind
func (r *AccessRequestV3) GetSubKind() string {
	return r.SubKind
}

// SetSubKind sets SubKind
func (r *AccessRequestV3) SetSubKind(subKind string) {
	r.SubKind = subKind
}

// GetVersion gets Version
func (r *AccessRequestV3) GetVersion() string {
	return r.Version
}

// GetName gets Name
func (r *AccessRequestV3) GetName() string {
	return r.Metadata.Name
}

// SetName sets Name
func (r *AccessRequestV3) SetName(name string) {
	r.Metadata.Name = name
}

// Expiry gets Expiry
func (r *AccessRequestV3) Expiry() time.Time {
	return r.Metadata.Expiry()
}

// SetExpiry sets Expiry
func (r *AccessRequestV3) SetExpiry(expiry time.Time) {
	r.Metadata.SetExpiry(expiry)
}

// SetTTL sets Expires header using the provided clock.
// Use SetExpiry instead.
// DELETE IN 7.0.0
func (r *AccessRequestV3) SetTTL(clock Clock, ttl time.Duration) {
	r.Metadata.SetTTL(clock, ttl)
}

// GetMetadata gets Metadata
func (r *AccessRequestV3) GetMetadata() Metadata {
	return r.Metadata
}

// GetResourceID gets ResourceID
func (r *AccessRequestV3) GetResourceID() int64 {
	return r.Metadata.GetID()
}

// SetResourceID sets ResourceID
func (r *AccessRequestV3) SetResourceID(id int64) {
	r.Metadata.SetID(id)
}

// String returns a text representation of this AccessRequest
func (r *AccessRequestV3) String() string {
	return fmt.Sprintf("AccessRequest(user=%v,roles=%+v)", r.Spec.User, r.Spec.Roles)
}

// Equals compares two AccessRequests
func (r *AccessRequestV3) Equals(other AccessRequest) bool {
	o, ok := other.(*AccessRequestV3)
	if !ok {
		return false
	}
	if r.GetName() != o.GetName() {
		return false
	}
	return r.Spec.Equals(&o.Spec)
}

// MatchesFilter returns true if Filter rule matches
// Empty Filter block always matches
func (t AccessReviewThreshold) MatchesFilter(parser predicate.Parser) (bool, error) {
	if t.Filter == "" {
		return true, nil
	}
	ifn, err := parser.Parse(t.Filter)
	if err != nil {
		return false, trace.Wrap(err)
	}
	fn, ok := ifn.(predicate.BoolPredicate)
	if !ok {
		return false, trace.BadParameter("unsupported type: %T", ifn)
	}
	return fn(), nil
}

func (t AccessReviewThreshold) Equals(other AccessReviewThreshold) bool {
	return reflect.DeepEqual(t, other)
}

func (c AccessReviewConditions) IsZero() bool {
	return reflect.ValueOf(c).IsZero()
}

func (c AccessRequestConditions) IsZero() bool {
	return reflect.ValueOf(c).IsZero()
}

func (s AccessReviewSubmission) Check() error {
	if s.RequestID == "" {
		return trace.BadParameter("missing request ID")
	}

	return s.Review.Check()
}

func (s AccessReview) Check() error {
	if s.Author == "" {
		return trace.BadParameter("missing review author")
	}

	return nil
}

// AccessRequestUpdate encompasses the parameters of a
// SetAccessRequestState call.
type AccessRequestUpdate struct {
	// RequestID is the ID of the request to be updated.
	RequestID string
	// State is the state that the target request
	// should resolve to.
	State RequestState
	// Reason is an optional description of *why* the
	// the request is being resolved.
	Reason string
	// Annotations supplies extra data associated with
	// the resolution; primarily for audit purposes.
	Annotations map[string][]string
	// Roles, if non-empty declares a list of roles
	// that should override the role list of the request.
	// This parameter is only accepted on approvals
	// and must be a subset of the role list originally
	// present on the request.
	Roles []string
}

// Check validates the request's fields
func (u *AccessRequestUpdate) Check() error {
	if u.RequestID == "" {
		return trace.BadParameter("missing request id")
	}
	if u.State.IsNone() {
		return trace.BadParameter("missing request state")
	}
	if len(u.Roles) > 0 && !u.State.IsApproved() {
		return trace.BadParameter("cannot override roles when setting state: %s", u.State)
	}
	return nil
}

// RequestStrategy is an indicator of how access requests
// should be handled for holders of a given role.
type RequestStrategy string

const (
	// RequestStrategyOptional is the default request strategy,
	// indicating that no special actions/requirements exist.
	RequestStrategyOptional RequestStrategy = "optional"

	// RequestStrategyReason indicates that client implementations
	// should automatically generate wildcard requests on login, and
	// users should be prompted for a reason.
	RequestStrategyReason RequestStrategy = "reason"

	// RequestStrategyAlways indicates that client implementations
	// should automatically generate wildcard requests on login, but
	// that reasons are not required.
	RequestStrategyAlways RequestStrategy = "always"
)

// ShouldAutoRequest checks if the request strategy
// indicates that a request should be automatically
// generated on login.
func (s RequestStrategy) ShouldAutoRequest() bool {
	switch s {
	case RequestStrategyReason, RequestStrategyAlways:
		return true
	default:
		return false
	}
}

// RequireReason checks if the request strategy
// is one that requires users to always supply
// reasons with their requests.
func (s RequestStrategy) RequireReason() bool {
	return s == RequestStrategyReason
}

// stateVariants allows iteration of the expected variants
// of RequestState.
var stateVariants = [4]RequestState{
	RequestState_NONE,
	RequestState_PENDING,
	RequestState_APPROVED,
	RequestState_DENIED,
}

// Parse attempts to interpret a value as a string representation
// of a RequestState.
func (s *RequestState) Parse(val string) error {
	for _, state := range stateVariants {
		if state.String() == val {
			*s = state
			return nil
		}
	}
	return trace.BadParameter("unknown request state: %q", val)
}

// IsNone request state
func (s RequestState) IsNone() bool {
	return s == RequestState_NONE
}

// IsPending request state
func (s RequestState) IsPending() bool {
	return s == RequestState_PENDING
}

// IsApproved request state
func (s RequestState) IsApproved() bool {
	return s == RequestState_APPROVED
}

// IsDenied request state
func (s RequestState) IsDenied() bool {
	return s == RequestState_DENIED
}

// IsResolved request state
func (s RequestState) IsResolved() bool {
	return s.IsApproved() || s.IsDenied()
}

// Equals compares two AccessRequestSpecs
func (s *AccessRequestSpecV3) Equals(other *AccessRequestSpecV3) bool {
	if s.User != other.User {
		return false
	}
	if len(s.Roles) != len(other.Roles) {
		return false
	}
	for i, role := range s.Roles {
		if role != other.Roles[i] {
			return false
		}
	}
	if s.Created != other.Created {
		return false
	}
	if s.Expires != other.Expires {
		return false
	}
	return s.State == other.State
}

// key values for map encoding of request filter
const (
	keyID    = "id"
	keyUser  = "user"
	keyState = "state"
)

// IntoMap copies AccessRequestFilter values into a map
func (f *AccessRequestFilter) IntoMap() map[string]string {
	m := make(map[string]string)
	if f.ID != "" {
		m[keyID] = f.ID
	}
	if f.User != "" {
		m[keyUser] = f.User
	}
	if !f.State.IsNone() {
		m[keyState] = f.State.String()
	}
	return m
}

// FromMap copies values from a map into this AccessRequestFilter value
func (f *AccessRequestFilter) FromMap(m map[string]string) error {
	for key, val := range m {
		switch key {
		case keyID:
			f.ID = val
		case keyUser:
			f.User = val
		case keyState:
			if err := f.State.Parse(val); err != nil {
				return trace.Wrap(err)
			}
		default:
			return trace.BadParameter("unknown filter key %s", key)
		}
	}
	return nil
}

// Match checks if a given access request matches this filter.
func (f *AccessRequestFilter) Match(req AccessRequest) bool {
	if f.ID != "" && req.GetName() != f.ID {
		return false
	}
	if f.User != "" && req.GetUser() != f.User {
		return false
	}
	if !f.State.IsNone() && req.GetState() != f.State {
		return false
	}
	return true
}

// Equals compares two AccessRequestFilters
func (f *AccessRequestFilter) Equals(o AccessRequestFilter) bool {
	return f.ID == o.ID && f.User == o.User && f.State == o.State
}
