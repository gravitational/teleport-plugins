package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"os"

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
	HTTP struct {
		Listen   string `toml:"listen"`
		KeyFile  string `toml:"https-key-file,omitempty"`
		CertFile string `toml:"https-cert-file,omitempty"`
	}
}

const exampleConfig = `# example slackbot configuration TOML file
[teleport]
auth-server = "example.com:3025"  # Auth GRPC API address
client-key = "path/to/client.key" # Teleport GRPC client secret key
client-crt = "path/to/client.crt" # Teleport GRPC client certificate
root-cas = "path/to/root.cas"     # Teleport cluster CA certs

[slack]
token = "api-token"       # Slack Bot OAuth token
secret = "signing-secret-value"   # Slack API Signing Secret
channel = "channel-name"  # Message delivery channel

[http]
listen = ":8081"          # Slack interaction callback listener
# https-key-file = "/var/lib/teleport/slackbot_key.pem"  # TLS private key
# https-cert-file = "/var/lib/teleport/slackbot_cert.pem" # TLS certificate
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
