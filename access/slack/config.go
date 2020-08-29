package main

import (
	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

// Config stores the full configuration for the teleport-slack plugin to run.
type Config struct {
	Teleport utils.TeleportConfig `toml:"teleport"`
	Slack    SlackConfig          `toml:"slack"`
	HTTP     utils.HTTPConfig     `toml:"http"`
	Log      utils.LogConfig      `toml:"log"`
}

// SlackConfig holds Slack-specific configuration options.
type SlackConfig struct {
	Token      string `toml:"token"`
	Secret     string `toml:"secret"`
	Channel    string `toml:"channel"`
	NotifyOnly bool   `toml:"notify_only"`
	APIURL     string
}

const exampleConfig = `# example slack plugin configuration TOML file
[teleport]
auth_server = "example.com:3025"                        # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/slack/auth.key" # Teleport GRPC client secret key
client_crt = "/var/lib/teleport/plugins/slack/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/slack/auth.cas"   # Teleport cluster CA certs

[slack]
token = "api_token"             # Slack Bot OAuth token
secret = "signing-secret-value" # Slack API Signing Secret
channel = "channel-name"        # Slack Channel name to post requests to
notify_only = false                # Allow Approval / Denial actions on Slack, or use it as notification only

[http]
public_addr = "example.com" # URL on which callback server is accessible externally, e.g. [https://]teleport-proxy.example.com
# listen_addr = ":8081" # Network address in format [addr]:port on which callback server listens, e.g. 0.0.0.0:8081
https_key_file = "/var/lib/teleport/webproxy_key.pem"  # TLS private key
https_cert_file = "/var/lib/teleport/webproxy_cert.pem" # TLS certificate

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
	if err := conf.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return conf, nil
}

// CheckAndSetDefaults checks the config struct for any logical errors, and sets default values
// if some values are missing.
// If critical values are missing and we can't set defaults for them — this will return an error.
func (c *Config) CheckAndSetDefaults() error {
	if c.Teleport.AuthServer == "" {
		c.Teleport.AuthServer = "localhost:3025"
	}
	if c.Teleport.ClientKey == "" {
		c.Teleport.ClientKey = "client.key"
	}
	if c.Teleport.ClientCrt == "" {
		c.Teleport.ClientCrt = "client.pem"
	}
	if c.Teleport.RootCAs == "" {
		c.Teleport.RootCAs = "cas.pem"
	}
	if c.Slack.Token == "" {
		return trace.BadParameter("missing required value slack.token")
	}
	if c.Slack.Secret == "" {
		return trace.BadParameter("missing required value slack.secret")
	}
	if c.Slack.Channel == "" {
		return trace.BadParameter("missing required value slack.channel")
	}
	if c.HTTP.ListenAddr == "" {
		c.HTTP.ListenAddr = ":8081"
	}
	if err := c.HTTP.Check(); err != nil {
		return trace.Wrap(err)
	}
	if c.Log.Output == "" {
		c.Log.Output = "stderr"
	}
	if c.Log.Severity == "" {
		c.Log.Severity = "info"
	}
	return nil
}
