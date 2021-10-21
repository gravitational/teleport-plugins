package main

import (
	"io/ioutil"
	"strings"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

// Config stores the full configuration for the teleport-slack plugin to run.
type Config struct {
	Teleport lib.TeleportConfig `toml:"teleport"`
	Slack    SlackConfig        `toml:"slack"`
	Log      logger.Config      `toml:"log"`
}

// SlackConfig holds Slack-specific configuration options.
type SlackConfig struct {
	Token      string   `toml:"token"`
	Recipients []string `toml:"recipients"`
	APIURL     string
}

const exampleConfig = `# Example slack plugin configuration TOML file

[teleport]
# Teleport Auth/Proxy Server address.
#
# Should be port 3025 for Auth Server and 3080 or 443 for Proxy.
# For Teleport Cloud, should be in the form "your-account.teleport.sh:443".
addr = "example.com:3025"

# Credentials.
#
# When using --format=file:
# identity = "/var/lib/teleport/plugins/slack/auth_id"    # Identity file
#
# When using --format=tls:
# client_key = "/var/lib/teleport/plugins/slack/auth.key" # Teleport TLS secret key
# client_crt = "/var/lib/teleport/plugins/slack/auth.crt" # Teleport TLS certificate
# root_cas = "/var/lib/teleport/plugins/slack/auth.cas"   # Teleport CA certs

[slack]
token = "xoxb-11xx"                                 # Slack Bot OAuth token
# recipients = ["person@email.com","YYYYYYY"]       # Optional Slack Rooms 
                                                    # Can also set suggested_reviewers for each role

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/slack.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
`

// LoadConfig reads the config file, initializes a new Config struct object, and returns it.
// Optionally returns an error if the file is not readable, or if file format is invalid.
func LoadConfig(filepath string) (*Config, error) {
	t, err := toml.LoadFile(filepath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	conf := &Config{}
	if err := t.Unmarshal(conf); err != nil {
		return nil, trace.Wrap(err)
	}
	if strings.HasPrefix(conf.Slack.Token, "/") {
		tokenBytes, err := ioutil.ReadFile(conf.Slack.Token)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		conf.Slack.Token = string(tokenBytes)
	}
	if err := conf.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return conf, nil
}

// CheckAndSetDefaults checks the config struct for any logical errors, and sets default values
// if some values are missing.
// If critical values are missing and we can't set defaults for them — this will return an error.
func (c *Config) CheckAndSetDefaults() error {
	if err := c.Teleport.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	if c.Slack.Token == "" {
		return trace.BadParameter("missing required value slack.token")
	}
	if c.Log.Output == "" {
		c.Log.Output = "stderr"
	}
	if c.Log.Severity == "" {
		c.Log.Severity = "info"
	}
	return nil
}
