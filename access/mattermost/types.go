package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gravitational/teleport-plugins/access"
)

// Plugin data

type RequestData struct {
	User          string
	Roles         []string
	RequestReason string
}

type MattermostData = []MattermostDataPost

type MattermostDataPost struct {
	PostID    string
	ChannelID string
}

type PluginData struct {
	RequestData
	MattermostData
}

// Mattermost API types

type Props map[string]interface{}

type Post struct {
	ID        string `json:"id,omitempty"`
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
	Props     Props  `json:"props"`
}

type Attachment struct {
	ID      int64        `json:"id"`
	Text    string       `json:"text"`
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

type User struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Channel struct {
	ID     string `json:"id"`
	TeamID string `json:"team_id"`
	Type   string `json:"type"`
	Name   string `json:"name"`
}

type ErrorResult struct {
	ID            string `json:"id"`
	Message       string `json:"message"`
	DetailedError string `json:"detailed_error"`
	RequestID     string `json:"request_id"`
	StatusCode    int    `json:"status_code"`
}

func (e ErrorResult) Error() string {
	return fmt.Sprintf("api error status_code=%v, message=%q", e.StatusCode, e.Message)
}

func DecodePluginData(dataMap access.PluginDataMap) (data PluginData) {
	data.User = dataMap["user"]
	data.Roles = strings.Split(dataMap["roles"], ",")
	data.RequestReason = dataMap["request_reason"]
	if channelID, postID := dataMap["channel_id"], dataMap["postID"]; channelID != "" && postID != "" {
		data.MattermostData = append(data.MattermostData, MattermostDataPost{ChannelID: channelID, PostID: postID})
	}
	for _, encodedMsg := range strings.Split(dataMap["messages"], ",") {
		if parts := strings.Split(encodedMsg, "/"); len(parts) == 2 {
			data.MattermostData = append(data.MattermostData, MattermostDataPost{ChannelID: parts[0], PostID: parts[1]})
		}
	}
	return
}

func EncodePluginData(data PluginData) access.PluginDataMap {
	var encodedMessages []string
	for _, msg := range data.MattermostData {
		encodedMessages = append(encodedMessages, fmt.Sprintf("%s/%s", msg.ChannelID, msg.PostID))
	}
	return access.PluginDataMap{
		"user":           data.User,
		"roles":          strings.Join(data.Roles, ","),
		"request_reason": data.RequestReason,
		"messages":       strings.Join(encodedMessages, ","),
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
