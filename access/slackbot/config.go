package main

import (
	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

type Config struct {
	Teleport struct {
		AuthServer string `toml:"auth-server"`
		ClientKey  string `toml:"client-key"`
		ClientCrt  string `toml:"client-crt"`
		RootCAs    string `toml:"root-cas"`
	} `toml:"teleport"`
	Slack struct {
		Token   string `toml:"token"`
		Secret  string `toml:"secret"`
		Channel string `toml:"channel"`
		APIURL  string
	} `toml:"slack"`
	HTTP utils.HTTPConfig `toml:"http"`
	Log  utils.LogConfig  `toml:"log"`
}

const exampleConfig = `# example slackbot configuration TOML file
[teleport]
auth-server = "example.com:3025"  # Teleport Auth Server GRPC API address
client-key = "/var/lib/teleport/plugins/slackbot/auth.key" # Teleport GRPC client secret key
client-crt = "/var/lib/teleport/plugins/slackbot/auth.crt" # Teleport GRPC client certificate
root-cas = "/var/lib/teleport/plugins/slackbot/auth.cas"   # Teleport cluster CA certs

[slack]
token = "api-token"       # Slack Bot OAuth token
secret = "signing-secret-value"   # Slack API Signing Secret
channel = "channel-name"  # Slack Channel name to post requests to

[http]
listen = ":8081"          # Slack interaction callback listener
# https-key-file = "/var/lib/teleport/plugins/slackbot/server.key"  # TLS private key
# https-cert-file = "/var/lib/teleport/plugins/slackbot/server.crt" # TLS certificate

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/slackbot.log"
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
	if c.HTTP.Listen == "" {
		c.HTTP.Listen = ":8081"
	}
	if c.HTTP.KeyFile != "" && c.HTTP.CertFile == "" {
		return trace.BadParameter("https-cert-file is required when https-key-file is specified")
	}
	if c.HTTP.CertFile != "" && c.HTTP.KeyFile == "" {
		return trace.BadParameter("https-key-file is required when https-cert-file is specified")
	}
	if c.Log.Output == "" {
		c.Log.Output = "stderr"
	}
	if c.Log.Severity == "" {
		c.Log.Severity = "info"
	}
	return nil
}
