package main

import (
	"strings"
)

type requestData struct {
	user  string
	roles []string
}

type slackData struct {
	channelID string
	timestamp string
}

type appData struct {
	requestData
	slackData
}

func (r *appData) UnmarshalPluginDataMap(data map[string]string) error {
	r.user = data["user"]
	r.roles = strings.Split(data["roles"], ",")
	r.channelID = data["channel_id"]
	r.timestamp = data["timestamp"]
	return nil
}

func (r *appData) MarshalPluginDataMap() (map[string]string, error) {
	return map[string]string{
		"user":       r.user,
		"roles":      strings.Join(r.roles, ","),
		"channel_id": r.channelID,
		"timestamp":  r.timestamp,
	}, nil
}
