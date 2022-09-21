package main

import (
	"github.com/gravitational/teleport-plugins/access/common"
)

const (
	// slackPluginName is used to tag Slack GenericPluginData and as a Delegator in Audit log.
	slackPluginName = "slack"
)

// NewSlackApp initializes a new teleport-slack app and returns it.
func NewSlackApp(conf SlackConfig) *common.BaseApp[SlackConfig] {
	return common.NewApp[SlackConfig](conf, slackPluginName, NewSlackBot)
}
