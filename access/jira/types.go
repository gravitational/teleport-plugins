package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gravitational/teleport-plugins/access"

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

func DecodePluginData(dataMap map[string]string) (data PluginData) {
	var created int64
	data.User = dataMap["user"]
	data.Roles = strings.Split(dataMap["roles"], ",")
	fmt.Sscanf(dataMap["created"], "%d", &created)
	data.Created = time.Unix(created, 0)
	data.ID = dataMap["issue_id"]
	data.Key = dataMap["issue_key"]
	return
}

func EncodePluginData(data PluginData) access.PluginDataMap {
	return access.PluginDataMap{
		"issue_id":  data.ID,
		"issue_key": data.Key,
		"user":      data.User,
		"roles":     strings.Join(data.Roles, ","),
		"created":   fmt.Sprintf("%d", data.Created.Unix()),
	}
}
