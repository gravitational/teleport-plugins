/*
Copyright 2015-2021 Gravitational, Inc.

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

	"github.com/stretchr/testify/assert"
)

var samplePluginData = PluginData{
	RequestData: RequestData{
		User:          "user-foo",
		Roles:         []string{"role-foo", "role-bar"},
		RequestReason: "foo reason",
		ReviewsCount:  3,
		Resolution:    Resolution{Tag: ResolvedApproved, Reason: "foo ok"},
	},
	EmailThreads: []EmailThread{
		{Email: "E1", MessageID: "M1", Timestamp: "0000001"},
		{Email: "E2", MessageID: "M2", Timestamp: "0000002"},
	},
}

func TestEncodePluginData(t *testing.T) {
	dataMap := EncodePluginData(samplePluginData)
	assert.Len(t, dataMap, 7)
	assert.Equal(t, "user-foo", dataMap["user"])
	assert.Equal(t, "role-foo,role-bar", dataMap["roles"])
	assert.Equal(t, "foo reason", dataMap["request_reason"])
	assert.Equal(t, "3", dataMap["reviews_count"])
	assert.Equal(t, "APPROVED", dataMap["resolution"])
	assert.Equal(t, "foo ok", dataMap["resolve_reason"])
	assert.Equal(t, "E1/0000001/M1,E2/0000002/M2", dataMap["email_threads"])
}

func TestDecodePluginData(t *testing.T) {
	pluginData := DecodePluginData(map[string]string{
		"user":           "user-foo",
		"roles":          "role-foo,role-bar",
		"request_reason": "foo reason",
		"reviews_count":  "3",
		"resolution":     "APPROVED",
		"resolve_reason": "foo ok",
		"email_threads":  "E1/0000001/M1,E2/0000002/M2",
	})
	assert.Equal(t, samplePluginData, pluginData)
}

func TestEncodeEmptyPluginData(t *testing.T) {
	dataMap := EncodePluginData(PluginData{})
	assert.Len(t, dataMap, 7)
	for key, value := range dataMap {
		assert.Emptyf(t, value, "value at key %q must be empty", key)
	}
}

func TestDecodeEmptyPluginData(t *testing.T) {
	assert.Empty(t, DecodePluginData(nil))
	assert.Empty(t, DecodePluginData(make(map[string]string)))
}
