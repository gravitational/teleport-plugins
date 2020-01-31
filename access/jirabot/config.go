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
	JIRA struct {
		URL   string `toml:"url"`
		Username  string `toml:"username"`
		APIToken string `toml:"api-token"`
		Project string `toml:"project"`
	} `toml:"jira"`
	HTTP utils.HTTPConfig `toml:"http"`
	Log  utils.LogConfig  `toml:"log"`
}

const exampleConfig = `# example jirabot configuration TOML file
[teleport]
auth-server = "example.com:3025"  # Auth GRPC API address
client-key = "/var/lib/teleport/plugins/jirabot/auth.key" # Teleport GRPC client secret key
client-crt = "/var/lib/teleport/plugins/jirabot/auth.crt" # Teleport GRPC client certificate
root-cas = "/var/lib/teleport/plugins/jirabot/auth.cas"   # Teleport cluster CA certs

[jira]
url = "https://example.com/jira"    # JIRA URL. For JIRA Cloud, https://[my-jira].atlassian.net
username = "bot@example.com"        # JIRA username
api-token = "token"                 # JIRA API Basic Auth token
project = "MYPROJ"                  # JIRA Project key

[http]
listen = ":8081"          # JIRA webhook listener
# host = "example.com"    # Host name by which bot is accessible
# https-key-file = "/var/lib/teleport/plugins/jirabot/server.key"  # TLS private key
# https-cert-file = "/var/lib/teleport/plugins/jirabot/server.crt" # TLS certificate

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/jirabot.log"
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
	if c.JIRA.URL == "" {
		return trace.BadParameter("missing required value jira.url")
	}
	if c.JIRA.Username == "" {
		return trace.BadParameter("missing required value jira.username")
	}
	if c.JIRA.APIToken == "" {
		return trace.BadParameter("missing required value jira.api-token")
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
