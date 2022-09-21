/*
Copyright 2022 Gravitational, Inc.

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

package common

import (
	"fmt"

	"github.com/gravitational/teleport-plugins/lib/stringset"
	"github.com/gravitational/teleport/api/types"
)

// RecipientsMap is a mapping of roles to recipient(s).
type RecipientsMap map[string][]string

// UnmarshalTOML will convert the input into map[string][]string
// The input can be one of the following:
// "key" = "value"
// "key" = ["multiple", "values"]
func (r *RecipientsMap) UnmarshalTOML(in interface{}) error {
	*r = make(RecipientsMap)

	recipientsMap, ok := in.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected type for recipients %T", in)
	}

	for k, v := range recipientsMap {
		switch val := v.(type) {
		case string:
			(*r)[k] = []string{val}
		case []interface{}:
			for _, str := range val {
				str, ok := str.(string)
				if !ok {
					return fmt.Errorf("unexpected type for recipients value %T", v)
				}
				(*r)[k] = append((*r)[k], str)
			}
		default:
			return fmt.Errorf("unexpected type for recipients value %T", v)
		}
	}

	return nil
}

// GetRecipientsFor will return the set of recipients given a list of roles and suggested reviewers.
// We create a unique list based on:
// - the list of suggestedReviewers
// - for each role, the list of reviewers
//   - if the role doesn't exist in the map (or it's empty), we add the list of recipients for the default role ("*") instead
func (r RecipientsMap) GetRecipientsFor(roles, suggestedReviewers []string) []string {
	recipients := stringset.New()

	for _, role := range roles {
		roleRecipients := r[role]
		if len(roleRecipients) == 0 {
			roleRecipients = r[types.Wildcard]
		}

		recipients.Add(roleRecipients...)
	}

	recipients.Add(suggestedReviewers...)

	return recipients.ToSlice()
}

// GetAllRecipients returns unique set of recipients
func (r RecipientsMap) GetAllRecipients() []string {
	recipients := stringset.New()

	for _, r := range r {
		recipients.Add(r...)
	}

	return recipients.ToSlice()
}
