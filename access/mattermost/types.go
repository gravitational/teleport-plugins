package main

import (
	"strings"

	"github.com/gravitational/teleport-plugins/access"

	log "github.com/sirupsen/logrus"
)

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

type logFields = log.Fields

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
