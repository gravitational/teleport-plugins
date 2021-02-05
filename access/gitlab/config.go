package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"os"
	"path"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

type Config struct {
	Teleport lib.TeleportConfig `toml:"teleport"`
	DB       struct {
		Path string `toml:"path"`
	} `toml:"db"`
	Gitlab GitlabConfig   `toml:"gitlab"`
	HTTP   lib.HTTPConfig `toml:"http"`
	Log    logger.Config  `toml:"log"`
}

type GitlabConfig struct {
	URL           string `toml:"url"`
	Token         string `toml:"token"`
	ProjectID     string `toml:"project_id"`
	WebhookSecret string `toml:"webhook_secret"`
}

const exampleConfig = `# example teleport-gitlab configuration TOML file
[teleport]
auth_server = "example.com:3025"                         # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/gitlab/auth.key" # Teleport GRPC API client secret key
client_crt = "/var/lib/teleport/plugins/gitlab/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/gitlab/auth.cas"   # Teleport cluster CA certs

[db]
path = "/var/lib/teleport/plugins/gitlab/database" # Path to the database file

[gitlab]
url = ""                                   # Leave empty if you are using cloud
token = "token"                            # GitLab API Token
project_id = "1812345"                     # GitLab Project ID
webhook_secret = "your webhook passphrase" # A secret used to encrypt data we use in webhooks. Basically anything you'd like.

[http]
public_addr = "example.com" # URL on which callback server is accessible externally, e.g. [https://]teleport-proxy.example.com
# listen_addr = ":8081" # Network address in format [addr]:port on which callback server listens, e.g. 0.0.0.0:8081
https_key_file = "/var/lib/teleport/webproxy_key.pem"  # TLS private key
https_cert_file = "/var/lib/teleport/webproxy_cert.pem" # TLS certificate

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/gitlab.log"
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
	if c.DB.Path == "" {
		c.DB.Path = path.Join(DefaultDir, "/database")
	}
	if c.Gitlab.Token == "" {
		return trace.BadParameter("missing required value gitlab.token")
	}
	if c.Gitlab.ProjectID == "" {
		return trace.BadParameter("missing required value gitlab.project_id")
	}
	if c.Gitlab.WebhookSecret == "" {
		return trace.BadParameter("missing required value gitlab.webhook_secret")
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
