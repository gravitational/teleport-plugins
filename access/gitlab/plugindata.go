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
	"strings"
	"time"

	"github.com/gravitational/teleport-plugins/lib/plugindata"
)

// PluginData is a data associated with access request that we store in Teleport using UpdatePluginData API.
type PluginData struct {
	RequestData
	GitlabData
}

type Resolution struct {
	Tag    ResolutionTag
	Reason string
}
type ResolutionTag string

const Unresolved = ResolutionTag("")
const ResolvedApproved = ResolutionTag("approved")
const ResolvedDenied = ResolutionTag("denied")
const ResolvedExpired = ResolutionTag("expired")

type RequestData struct {
	User          string
	Roles         []string
	Created       time.Time
	RequestReason string
	ReviewsCount  int
	Resolution    Resolution
}

type GitlabData struct {
	IssueID   IntID
	IssueIID  IntID
	ProjectID IntID
}

// UnmarshalPluginData deserializes a string map to PluginData struct.
func (data *PluginData) UnmarshalPluginData(dataMap plugindata.StringMap) {
	data.User = dataMap["user"]
	data.Roles = plugindata.SplitString(dataMap["roles"], ",")
	data.Created = plugindata.DecodeTime(dataMap["created"])
	data.RequestReason = dataMap["request_reason"]
	data.ReviewsCount = plugindata.DecodeInt(dataMap["reviews_count"])
	data.Resolution.Tag = ResolutionTag(dataMap["resolution"])
	data.Resolution.Reason = dataMap["resolve_reason"]
	data.ProjectID = decodeIntID(dataMap["project_id"])
	data.IssueIID = decodeIntID(dataMap["issue_iid"])
	data.IssueID = decodeIntID(dataMap["issue_id"])
}

// MarshalPluginData serializes a PluginData struct into a string map.
func (data *PluginData) MarshalPluginData() plugindata.StringMap {
	if data == nil {
		data = &PluginData{}
	}
	return plugindata.StringMap{
		"user":           data.User,
		"roles":          strings.Join(data.Roles, ","),
		"created":        plugindata.EncodeTime(data.Created),
		"request_reason": data.RequestReason,
		"reviews_count":  plugindata.EncodeInt(data.ReviewsCount),
		"resolution":     string(data.Resolution.Tag),
		"resolve_reason": data.Resolution.Reason,
		"project_id":     encodeIntID(data.ProjectID),
		"issue_iid":      encodeIntID(data.IssueIID),
		"issue_id":       encodeIntID(data.IssueID),
	}
}

func decodeIntID(str string) IntID {
	return IntID(plugindata.DecodeUint64(str))
}

func encodeIntID(id IntID) string {
	return plugindata.EncodeUint64(uint64(id))
}
