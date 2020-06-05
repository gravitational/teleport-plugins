package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gravitational/teleport-plugins/access"

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

func DecodePluginData(dataMap access.PluginDataMap) (data PluginData) {
	var created int64
	data.User = dataMap["user"]
	data.Roles = strings.Split(dataMap["roles"], ",")
	fmt.Sscanf(dataMap["created"], "%d", &created)
	data.Created = time.Unix(created, 0)
	data.ID = dataMap["incident_id"]
	return
}

func EncodePluginData(data PluginData) access.PluginDataMap {
	return access.PluginDataMap{
		"incident_id": data.ID,
		"user":        data.User,
		"roles":       strings.Join(data.Roles, ","),
		"created":     fmt.Sprintf("%d", data.Created.Unix()),
	}
}
