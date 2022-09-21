package main

import (
	"github.com/gravitational/teleport-plugins/access/common"
)

const (
	// discordPluginName is used to tag Discord GenericPluginData and as a Delegator in Audit log.
	discordPluginName = "discord"
)

// NewDiscordApp initializes a new teleport-discord app and returns it.
func NewDiscordApp(conf DiscordConfig) *common.BaseApp[DiscordConfig] {
	return common.NewApp[DiscordConfig](conf, discordPluginName, NewDiscordBot)
}
