package main

import (
	log "github.com/sirupsen/logrus"
)

type RequestData struct {
	User  string
	Roles []string
}

type SlackData struct {
	ChannelID string
	Timestamp string
}

type PluginData struct {
	RequestData
	SlackData
}

type logFields = log.Fields
