package main

import (
	"time"

	log "github.com/sirupsen/logrus"
)

type logFields = log.Fields

type RequestData struct {
	User    string
	Roles   []string
	Created time.Time
}

type PagerdutyData struct {
	ID string
}

type PluginData struct {
	RequestData
	PagerdutyData
}
