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

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var samplePluginData = PluginData{
	RequestData: RequestData{
		User:          "user-foo",
		Roles:         []string{"role-foo", "role-bar"},
		Created:       time.Date(2021, 6, 1, 13, 27, 17, 0, time.UTC).Local(),
		RequestReason: "foo reason",
		ReviewsCount:  3,
		Resolution:    Resolution{Tag: ResolvedApproved, Reason: "foo ok"},
	},
	GitlabData: GitlabData{
		ProjectID: IntID(333),
		IssueID:   IntID(1111),
		IssueIID:  IntID(11),
	},
}

func TestEncodePluginData(t *testing.T) {
	dataMap := EncodePluginData(samplePluginData)
	assert.Len(t, dataMap, 10)
	assert.Equal(t, "user-foo", dataMap["user"])
	assert.Equal(t, "role-foo,role-bar", dataMap["roles"])
	assert.Equal(t, "1622554037", dataMap["created"])
	assert.Equal(t, "foo reason", dataMap["request_reason"])
	assert.Equal(t, "3", dataMap["reviews_count"])
	assert.Equal(t, "approved", dataMap["resolution"])
	assert.Equal(t, "foo ok", dataMap["resolve_reason"])
	assert.Equal(t, "333", dataMap["project_id"])
	assert.Equal(t, "1111", dataMap["issue_id"])
	assert.Equal(t, "11", dataMap["issue_iid"])
}

func TestDecodePluginData(t *testing.T) {
	pluginData := DecodePluginData(map[string]string{
		"user":           "user-foo",
		"roles":          "role-foo,role-bar",
		"created":        "1622554037",
		"request_reason": "foo reason",
		"reviews_count":  "3",
		"resolution":     "approved",
		"resolve_reason": "foo ok",
		"project_id":     "333",
		"issue_id":       "1111",
		"issue_iid":      "11",
	})
	assert.Equal(t, samplePluginData, pluginData)
}

func TestEncodeEmptyPluginData(t *testing.T) {
	dataMap := EncodePluginData(PluginData{})
	assert.Len(t, dataMap, 10)
	for key, value := range dataMap {
		assert.Emptyf(t, value, "value at key %q must be empty", key)
	}
}

func TestDecodeEmptyPluginData(t *testing.T) {
	assert.Empty(t, DecodePluginData(nil))
	assert.Empty(t, DecodePluginData(make(map[string]string)))
}
