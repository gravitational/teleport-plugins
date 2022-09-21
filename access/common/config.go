package common

import (
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
)

type PluginConfiguration interface {
	GetTeleportConfig() lib.TeleportConfig
	GetRecipients() RecipientsMap
}

func (c BaseConfig) GetRecipients() RecipientsMap {
	return c.Recipients
}

func (c BaseConfig) GetTeleportConfig() lib.TeleportConfig {
	return c.Teleport
}

type BaseConfig struct {
	Teleport   lib.TeleportConfig
	Recipients RecipientsMap `toml:"role_to_recipients"`
	Log        logger.Config
}

// GenericAPIConfig holds Slack-specific configuration options.
type GenericAPIConfig struct {
	Token string
	// DELETE IN 11.0.0 (Joerger) - use "role_to_recipients["*"]" instead
	Recipients []string
	APIURL     string
}
