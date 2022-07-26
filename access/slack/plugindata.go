package main

import (
	"fmt"
	"strings"

	"github.com/gravitational/teleport-plugins/lib/plugindata"
)

// PluginData is a data associated with access request that we store in Teleport using UpdatePluginData API.
type PluginData struct {
	plugindata.AccessRequestData
	SlackData
}

type SlackDataMessage struct {
	ChannelID            string
	TimestampOrDiscordID string
}

type SlackData = []SlackDataMessage

// DecodePluginData deserializes a string map to PluginData struct.
func DecodePluginData(dataMap map[string]string) PluginData {
	data := PluginData{}

	data.AccessRequestData = plugindata.DecodeAccessRequestData(dataMap)

	if channelID, timestamp := dataMap["channel_id"], dataMap["timestamp"]; channelID != "" && timestamp != "" {
		data.SlackData = append(data.SlackData, SlackDataMessage{ChannelID: channelID, TimestampOrDiscordID: timestamp})
	}
	if str := dataMap["messages"]; str != "" {
		for _, encodedMsg := range strings.Split(str, ",") {
			if parts := strings.Split(encodedMsg, "/"); len(parts) == 2 {
				data.SlackData = append(data.SlackData, SlackDataMessage{ChannelID: parts[0], TimestampOrDiscordID: parts[1]})
			}
		}
	}
	return data
}

// EncodePluginData serializes a PluginData struct into a string map.
func EncodePluginData(data PluginData) map[string]string {
	result := plugindata.EncodeAccessRequestData(data.AccessRequestData)

	var encodedMessages []string
	for _, msg := range data.SlackData {
		encodedMessages = append(encodedMessages, fmt.Sprintf("%s/%s", msg.ChannelID, msg.TimestampOrDiscordID))
	}
	result["messages"] = strings.Join(encodedMessages, ",")

	return result
}
