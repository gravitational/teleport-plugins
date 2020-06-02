package main

import (
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
