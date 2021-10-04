/*
Copyright 2020-2021 Gravitational, Inc.

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

	"github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/stretchr/testify/require"
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
	PagerdutyData: PagerdutyData{
		ServiceID:  "SERVICE1",
		IncidentID: "INCIDENT1",
	},
}

var sampleStringMap = plugindata.StringMap{
	"user":           "user-foo",
	"roles":          "role-foo,role-bar",
	"created":        "1622554037",
	"request_reason": "foo reason",
	"reviews_count":  "3",
	"resolution":     "approved",
	"resolve_reason": "foo ok",
	"service_id":     "SERVICE1",
	"incident_id":    "INCIDENT1",
}

func TestMarshalPluginData(t *testing.T) {
	require.Equal(t, sampleStringMap, samplePluginData.MarshalPluginData())
}

func TestUnmarshalPluginData(t *testing.T) {
	var data PluginData
	data.UnmarshalPluginData(sampleStringMap)
	require.Equal(t, samplePluginData, data)
}

func TestMarshalEmptyPluginData(t *testing.T) {
	data := &PluginData{}
	dataMap := data.MarshalPluginData()
	require.Len(t, dataMap, 9)
	for key, value := range dataMap {
		require.Zerof(t, value, "value at key %q must be a zero", key)
	}
}

func TestUnmarshalEmptyPluginData(t *testing.T) {
	var data PluginData

	data.UnmarshalPluginData(nil)
	require.Zero(t, data)

	data.UnmarshalPluginData(make(map[string]string))
	require.Zero(t, data)
}
