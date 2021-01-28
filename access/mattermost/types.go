package main

import (
	"encoding/json"
	"strings"

	"github.com/gravitational/teleport-plugins/access"

	log "github.com/sirupsen/logrus"
)

type logFields = log.Fields

// Plugin data

type RequestData struct {
	User  string
	Roles []string
}

type MattermostData struct {
	PostID    string
	ChannelID string
}

type PluginData struct {
	RequestData
	MattermostData
}

// Mattermost API types

type Post struct {
	ID        string                 `json:"id"`
	ChannelID string                 `json:"channel_id"`
	Message   string                 `json:"message"`
	Props     map[string]interface{} `json:"props"`
}

type Attachment struct {
	ID      int64        `json:"id"`
	Actions []PostAction `json:"actions,omitempty"`
}

type PostAction struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Integration *PostActionIntegration `json:"integration,omitempty"`
}

type PostActionIntegration struct {
	URL     string                 `json:"url,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
}

func DecodePluginData(dataMap access.PluginDataMap) (data PluginData) {
	data.User = dataMap["user"]
	data.Roles = strings.Split(dataMap["roles"], ",")
	data.PostID = dataMap["post_id"]
	data.ChannelID = dataMap["channel_id"]
	return
}

func EncodePluginData(data PluginData) access.PluginDataMap {
	return access.PluginDataMap{
		"user":       data.User,
		"roles":      strings.Join(data.Roles, ","),
		"post_id":    data.PostID,
		"channel_id": data.ChannelID,
	}
}

func (post Post) Attachments() []Attachment {
	var attachments []Attachment
	if slice, ok := post.Props["attachments"].([]interface{}); ok {
		for _, dec := range slice {
			if enc, err := json.Marshal(dec); err == nil {
				var attachment Attachment
				if json.Unmarshal(enc, &attachment) == nil {
					attachments = append(attachments, attachment)
				}
			}
		}
	}
	return attachments
}
