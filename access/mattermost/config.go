package main

import (
	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

type Config struct {
	Teleport struct {
		AuthServer string `toml:"auth_server"`
		ClientKey  string `toml:"client_key"`
		ClientCrt  string `toml:"client_crt"`
		RootCAs    string `toml:"root_cas"`
	} `toml:"teleport"`
	Mattermost MattermostConfig `toml:"mattermost"`
	HTTP       utils.HTTPConfig `toml:"http"`
	Log        utils.LogConfig  `toml:"log"`
}

type MattermostConfig struct {
	URL     string `toml:"url"`
	Team    string `toml:"team"`
	Channel string `toml:"channel"`
	Token   string `toml:"token"`
	Secret  string `toml:"secret"`
}

const exampleConfig = `# example mattermost configuration TOML file
[teleport]
auth_server = "example.com:3025"                             # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/mattermost/auth.key" # Teleport GRPC client secret key
client_crt = "/var/lib/teleport/plugins/mattermost/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/mattermost/auth.cas"   # Teleport cluster CA certs

[mattermost]
url = "https://mattermost.example.com" # Mattermost Server URL
team = "team-name"                     # Mattermost team in which the channel resides.
channel = "channel-name"               # Mattermost Channel name to post requests to
token = "api-token"                    # Mattermost Bot OAuth token
secret = "signing-secret-value"        # Mattermost API signing Secret

[http]
public_addr = "example.com" # URL on which callback server is accessible externally, e.g. [https://]teleport-proxy.example.com
# listen_addr = ":8081" # Network address in format [addr]:port on which callback server listens, e.g. 0.0.0.0:8081
https_key_file = "/var/lib/teleport/webproxy_key.pem"  # TLS private key
https_cert_file = "/var/lib/teleport/webproxy_cert.pem" # TLS certificate

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
	if c.Mattermost.Token == "" {
		return trace.BadParameter("missing required value mattermost.token")
	}
	if c.Mattermost.Secret == "" {
		return trace.BadParameter("missing required value mattermost.secret")
	}
	if c.Mattermost.Team == "" {
		return trace.BadParameter("missing required value mattermost.team")
	}
	if c.Mattermost.Channel == "" {
		return trace.BadParameter("missing required value mattermost.channel")
	}
	if c.Mattermost.URL == "" {
		return trace.BadParameter("missing required value mattermost.url")
	}
	if c.HTTP.PublicAddr == "" {
		return trace.BadParameter("missing required value http.public_addr")
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
