package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"os"

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
	Pagerduty struct {
		APIEndpoint string `toml:"-"`
		APIKey      string `toml:"api-key"`
		UserEmail   string `toml:"user-email"`
		ServiceId   string `toml:"service-id"`
	} `toml:"pagerduty"`
	HTTP utils.HTTPConfig `toml:"http"`
	Log  utils.LogConfig  `toml:"log"`
}

const exampleConfig = `# example teleport-pagerduty configuration TOML file
[teleport]
auth-server = "example.com:3025"  # Auth GRPC API address
client-key = "/var/lib/teleport/plugins/pagerduty/auth.key" # Teleport GRPC client secret key
client-crt = "/var/lib/teleport/plugins/pagerduty/auth.crt" # Teleport GRPC client certificate
root-cas = "/var/lib/teleport/plugins/pagerduty/auth.cas"   # Teleport cluster CA certs

[pagerduty]
api-key = "key"               # PagerDuty API Key
user-email = "me@example.com" # PagerDuty bot user email (Could be admin email)
service-id = "PIJ90N7"        # PagerDuty service id

[http]
listen = ":8081"          # PagerDuty webhook listener
base-url = "https://teleport-pagerduty.infra.yourcorp.com" # The public address of the teleport-pagerduty webhook listener. 
# host = "example.com"    # Host name by which bot is accessible
# https-key-file = "/var/lib/teleport/plugins/pagerduty/server.key"  # TLS private key
# https-cert-file = "/var/lib/teleport/plugins/pagerduty/server.crt" # TLS certificate

[http.tls]
verify-client-cert = true # The preferred way to authenticate webhooks on Pagerduty. See more: https://developer.pagerduty.com/docs/webhooks/webhooks-mutual-tls

[http.basic-auth]
user = "user"
password = "password" # If you prefer to use basic auth for Pagerduty Webhooks authentication, use this section to store user and password

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/pagerduty.log"
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
	if c.Pagerduty.APIKey == "" {
		return trace.BadParameter("missing required value pagerduty.api-key")
	}
	if c.Pagerduty.UserEmail == "" {
		return trace.BadParameter("missing required value pagerduty.user-email")
	}
	if c.Pagerduty.ServiceId == "" {
		return trace.BadParameter("missing required value pagerduty.service-id")
	}
	if c.HTTP.Hostname == "" && c.HTTP.RawBaseURL == "" {
		return trace.BadParameter("either http.base-url or http.host is required to be set")
	}
	if c.HTTP.Listen == "" {
		c.HTTP.Listen = ":8081"
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

// LoadTLSConfig loads client crt/key files and root authorities, and
// generates a tls.Config suitable for use with a GRPC client.
func (c *Config) LoadTLSConfig() (*tls.Config, error) {
	var tc tls.Config
	clientCert, err := tls.LoadX509KeyPair(c.Teleport.ClientCrt, c.Teleport.ClientKey)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	tc.Certificates = append(tc.Certificates, clientCert)
	caFile, err := os.Open(c.Teleport.RootCAs)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	caCerts, err := ioutil.ReadAll(caFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(caCerts); !ok {
		return nil, trace.BadParameter("invalid CA cert PEM")
	}
	tc.RootCAs = pool
	return &tc, nil
}
