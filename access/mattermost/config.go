package main

import (
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

type Config struct {
	Teleport   lib.TeleportConfig `toml:"teleport"`
	Mattermost MattermostConfig   `toml:"mattermost"`
	Log        logger.Config      `toml:"log"`
}

type MattermostConfig struct {
	URL        string   `toml:"url"`
	Recipients []string `toml:"recipients"`
	Token      string   `toml:"token"`
}

const exampleConfig = `# example mattermost configuration TOML file
[teleport]
auth_server = "example.com:3025"                             # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/mattermost/auth.key" # Teleport GRPC client secret key
client_crt = "/var/lib/teleport/plugins/mattermost/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/mattermost/auth.cas"   # Teleport cluster CA certs

[mattermost]
url = "https://mattermost.example.com" # Mattermost Server URL
token = "api-token"                    # Mattermost Bot OAuth token

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/mattermost.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
`

func LoadConfig(filepath string) (*Config, error) {
	t, err := toml.LoadFile(filepath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	conf := &Config{}
	if err := t.Unmarshal(conf); err != nil {
		return nil, trace.Wrap(err)
	}
	if err := conf.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return conf, nil
}

func (c *Config) CheckAndSetDefaults() error {
	if c.Mattermost.Token == "" {
		return trace.BadParameter("missing required value mattermost.token")
	}
	if c.Mattermost.URL == "" {
		return trace.BadParameter("missing required value mattermost.url")
	}
	if c.Log.Output == "" {
		c.Log.Output = "stderr"
	}
	if c.Log.Severity == "" {
		c.Log.Severity = "info"
	}
	return nil
}
