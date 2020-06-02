package main

import (
	"time"

	log "github.com/sirupsen/logrus"
)

type RequestData struct {
	User    string
	Roles   []string
	Created time.Time
}

type JiraData struct {
	ID  string
	Key string
}

type PluginData struct {
	RequestData
	JiraData
}

type logFields = log.Fields
