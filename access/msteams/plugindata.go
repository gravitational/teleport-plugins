package main

import (
	"encoding/base64"
	"encoding/json"
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
	ID          string `json:"id"`
	Timestamp   string `json:"ts"`
	RecipientID string `json:"rid"`
}

// DecodePluginData deserializes a string map to PluginData struct.
func DecodePluginData(dataMap map[string]string) PluginData {
	data := PluginData{}

	data.AccessRequestData = plugindata.DecodeAccessRequestData(dataMap)

	if str := dataMap["messages"]; str != "" {
		for _, encodedMsg := range strings.Split(str, ",") {
			decodedMsg, err := base64.StdEncoding.DecodeString(encodedMsg)
			if err != nil {
				// Backward compatibility
				parts := strings.Split(encodedMsg, "/")
				if len(parts) == 3 {
					data.TeamsData = append(data.TeamsData, TeamsMessage{ID: parts[0], Timestamp: parts[1], RecipientID: parts[2]})
				}
			} else {
				msg := &TeamsMessage{}
				err = json.Unmarshal(decodedMsg, msg)
				if err != nil {
					continue
				}
				data.TeamsData = append(data.TeamsData, *msg)
			}
		}
	}

	return data
}

// EncodePluginData serializes plugin data to a string map
func EncodePluginData(data PluginData) map[string]string {
	result := plugindata.EncodeAccessRequestData(data.AccessRequestData)

	var encodedMessages []string
	for _, msg := range data.TeamsData {
		jsonMessage, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		encodedMessage := base64.StdEncoding.EncodeToString(jsonMessage)
		encodedMessages = append(encodedMessages, encodedMessage)
	}

	result["messages"] = strings.Join(encodedMessages, ",")

	return result
}
