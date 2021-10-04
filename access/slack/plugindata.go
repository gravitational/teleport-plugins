package main

import (
	"fmt"
	"strings"

	"github.com/gravitational/teleport-plugins/lib/plugindata"
)

// PluginData is a data associated with access request that we store in Teleport using UpdatePluginData API.
type PluginData struct {
	RequestData
	SlackData
}

type Resolution struct {
	Tag    ResolutionTag
	Reason string
}
type ResolutionTag string

const Unresolved = ResolutionTag("")
const ResolvedApproved = ResolutionTag("APPROVED")
const ResolvedDenied = ResolutionTag("DENIED")
const ResolvedExpired = ResolutionTag("EXPIRED")

type RequestData struct {
	User          string
	Roles         []string
	RequestReason string
	ReviewsCount  int
	Resolution    Resolution
}

type SlackDataMessage struct {
	ChannelID string
	Timestamp string
}

type SlackData = []SlackDataMessage

// UnmarshalPluginData deserializes a string map to PluginData struct.
func (data *PluginData) UnmarshalPluginData(dataMap plugindata.StringMap) {
	data.User = dataMap["user"]
	data.Roles = plugindata.SplitString(dataMap["roles"], ",")
	data.RequestReason = dataMap["request_reason"]
	data.ReviewsCount = plugindata.DecodeInt(dataMap["reviews_count"])
	data.Resolution.Tag = ResolutionTag(dataMap["resolution"])
	data.Resolution.Reason = dataMap["resolve_reason"]
	data.SlackData = decodeMessages(dataMap["messages"])
}

// MarshalPluginData serializes a PluginData struct into a string map.
func (data *PluginData) MarshalPluginData() plugindata.StringMap {
	if data == nil {
		data = &PluginData{}
	}
	return plugindata.StringMap{
		"user":           data.User,
		"roles":          strings.Join(data.Roles, ","),
		"request_reason": data.RequestReason,
		"reviews_count":  plugindata.EncodeInt(data.ReviewsCount),
		"resolution":     string(data.Resolution.Tag),
		"resolve_reason": data.Resolution.Reason,
		"messages":       encodeMessages(data.SlackData),
	}
}

func decodeMessages(str string) []SlackDataMessage {
	if str == "" {
		return nil
	}

	parts := strings.Split(str, ",")
	result := make([]SlackDataMessage, 0, len(parts))
	for _, part := range parts {
		if msgParts := strings.Split(part, "/"); len(msgParts) == 2 {
			result = append(result, SlackDataMessage{ChannelID: msgParts[0], Timestamp: msgParts[1]})
		}
	}
	return result
}

func encodeMessages(messages []SlackDataMessage) string {
	if len(messages) == 0 {
		return ""
	}

	encodedMessages := make([]string, len(messages))
	for i, msg := range messages {
		encodedMessages[i] = fmt.Sprintf("%s/%s", msg.ChannelID, msg.Timestamp)
	}
	return strings.Join(encodedMessages, ",")
}
