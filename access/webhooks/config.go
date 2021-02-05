package main

import (
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

// Config stores the full configuration for the teleport-slack plugin to run.
type Config struct {
	Teleport lib.TeleportConfig `toml:"teleport"`
	Webhook  WebhookConfig      `toml:"webhook"`
	HTTP     lib.HTTPConfig     `toml:"http"`
	Log      logger.Config      `toml:"log"`
}

// WebhookConfig represents webhook configuration section, including the URL to use and notifyOnly mode
type WebhookConfig struct {
	URL            string          `toml:"webhook_url"`
	NotifyOnly     bool            `toml:"notify_only"`
	RequestStates  map[string]bool `toml:"request_statuses"`
	CallbackSecret string          `toml:"callback_secret"`
}

const exampleConfig = `# example webhooks plugin configuration TOML file
[teleport]
auth_server = "example.com:3025"                        # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/webhooks/auth.key" # Teleport GRPC client secret key
client_crt = "/var/lib/teleport/plugins/webhooks/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/webhooks/auth.cas"   # Teleport cluster CA certs

[webhook]
webhook_url = "https://mywebhook.com/ppst" # Receiver webhook URL
notify_only = false # Allow Approval / Denial actions via the Callbacks
request_states = { "Pending" = true, "Approved" = false, "Denied" = false } # What request statuses to notify about?
callback_secret = "secret string to sign callback payloads with"

[http]
public_addr = "example.com" # URL on which callback server is accessible externally, e.g. [https://]teleport-proxy.example.com
# listen_addr = ":8081" # Network address in format [addr]:port on which callback server listens, e.g. 0.0.0.0:8081
https_key_file = "/var/lib/teleport/webproxy_key.pem"  # TLS private key
https_cert_file = "/var/lib/teleport/webproxy_cert.pem" # TLS certificate

#[http.basic_auth]
#user = "user"
#password = "password" # If you prefer to use basic auth for Webhooks authentication, use this section to store user and password

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

	if c.Webhook.URL == "" {
		return trace.BadParameter("missing required value webhook.webhook_url")
	}
	if c.Webhook.RequestStates == nil {
		c.Webhook.RequestStates = defaultStates
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

var defaultStates = map[string]bool{
	"Pending":  true,
	"Approved": false,
	"Denied":   false,
}
