package main

import (
	"fmt"
	"strings"

	"github.com/gravitational/teleport-plugins/lib/plugindata"
)

// PluginData is a data associated with access request that we store in Teleport using UpdatePluginData API.
type PluginData struct {
	plugindata.AccessRequestData
	TeamsData []TeamsMessage
}

// TeamsMessage represents sent message information
type TeamsMessage struct {
	ID          string
	Timestamp   string
	RecipientID string
}

// DecodePluginData deserializes a string map to PluginData struct.
func DecodePluginData(dataMap map[string]string) PluginData {
	data := PluginData{}

	data.AccessRequestData = plugindata.DecodeAccessRequestData(dataMap)

	if channelID, timestamp := dataMap["channel_id"], dataMap["timestamp"]; channelID != "" && timestamp != "" {
		data.TeamsData = append(data.TeamsData, TeamsMessage{ID: channelID, Timestamp: timestamp})
	}
	if str := dataMap["messages"]; str != "" {
		for _, encodedMsg := range strings.Split(str, ",") {
			parts := strings.Split(encodedMsg, "/")
			if len(parts) == 3 {
				data.TeamsData = append(data.TeamsData, TeamsMessage{ID: parts[0], Timestamp: parts[1], RecipientID: parts[2]})
			}
		}
	}

	return data
}

// Encode serializes plugin data to a string map
func EncodePluginData(data PluginData) map[string]string {
	result := plugindata.EncodeAccessRequestData(data.AccessRequestData)

	var encodedMessages []string
	for _, msg := range data.TeamsData {
		encodedMessages = append(encodedMessages, fmt.Sprintf("%s/%s/%s", msg.ID, msg.Timestamp, msg.RecipientID))
	}

	result["messages"] = strings.Join(encodedMessages, ",")

	return result
}
