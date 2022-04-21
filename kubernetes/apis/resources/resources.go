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

package resources

import (
	"github.com/gravitational/trace"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
)

// ResourceError describes an error occurred while communicating with Teleport.
type ResourceError struct {
	Kind string `json:"kind,omitempty"`
	Text string `json:"text"`
}

// ResourceStatus is a base struct type for embedding into the specific resource status type.
type ResourceStatus struct {
	LastError *ResourceError `json:"lastError,omitempty"`
}

// SetLastError saves the error's kind and its text.
func (status *ResourceStatus) SetLastError(err error) {
	switch {
	case err == nil:
		status.LastError = nil
	case trace.IsConnectionProblem(err):
		status.LastError = &ResourceError{Kind: "ConnectionProblem", Text: err.Error()}
	case trace.IsAccessDenied(err) || kerrors.IsForbidden(trace.Unwrap(err)):
		status.LastError = &ResourceError{Kind: "AccessDenied", Text: err.Error()}
	case trace.IsNotFound(err) || kerrors.IsNotFound(trace.Unwrap(err)):
		status.LastError = &ResourceError{Kind: "NotFound", Text: err.Error()}
	case trace.IsAlreadyExists(err):
		status.LastError = &ResourceError{Kind: "AlreadyExists", Text: err.Error()}
	case trace.IsBadParameter(err):
		status.LastError = &ResourceError{Kind: "BadParameter", Text: err.Error()}
	case trace.IsCompareFailed(err):
		status.LastError = &ResourceError{Kind: "CompareFailed", Text: err.Error()}
	default:
		status.LastError = &ResourceError{Text: err.Error()}
	}
}
