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
	Teleport  utils.TeleportConfig `toml:"teleport"`
	Pagerduty PagerdutyConfig      `toml:"pagerduty"`
	HTTP      utils.HTTPConfig     `toml:"http"`
	Log       utils.LogConfig      `toml:"log"`
}

type PagerdutyConfig struct {
	APIEndpoint string `toml:"-"`
	APIKey      string `toml:"api_key"`
	UserEmail   string `toml:"user_email"`
	ServiceID   string `toml:"service_id"`
	AutoApprove bool   `toml:"auto_approve"`
}

const exampleConfig = `# example teleport-pagerduty configuration TOML file
[teleport]
auth_server = "example.com:3025"                            # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/pagerduty/auth.key" # Teleport GRPC client secret key
client_crt = "/var/lib/teleport/plugins/pagerduty/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/pagerduty/auth.cas"   # Teleport cluster CA certs

[pagerduty]
api_key = "key"               # PagerDuty API Key
user_email = "me@example.com" # PagerDuty bot user email (Could be admin email)
service_id = "PIJ90N7"        # PagerDuty service id
auto_approve = true           # Automatically approve access requests if requestor is on-call

[http]
public_addr = "example.com:8081" # URL on which callback server is accessible externally, e.g. [https://]teleport-proxy.example.com:8081
# listen_addr = ":8081" # Network address in format [addr]:port on which callback server listens, e.g. 0.0.0.0:8081
https_key_file = "/var/lib/teleport/webproxy_key.pem"  # TLS private key
https_cert_file = "/var/lib/teleport/webproxy_cert.pem" # TLS certificate

[http.tls]
verify_client_cert = true # The preferred way to authenticate webhooks on Pagerduty. See more: https://developer.pagerduty.com/docs/webhooks/webhooks-mutual-tls

#[http.basic_auth]
#user = "user"
#password = "password" # If you prefer to use basic auth for Pagerduty Webhooks authentication, use this section to store user and password

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
		return trace.BadParameter("missing required value pagerduty.api_key")
	}
	if c.Pagerduty.UserEmail == "" {
		return trace.BadParameter("missing required value pagerduty.user_email")
	}
	if c.Pagerduty.ServiceID == "" {
		return trace.BadParameter("missing required value pagerduty.service_id")
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
