package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"os"
	"strings"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

type Config struct {
	Teleport lib.TeleportConfig `toml:"teleport"`
	JIRA     JIRAConfig         `toml:"jira"`
	HTTP     lib.HTTPConfig     `toml:"http"`
	Log      logger.Config      `toml:"log"`
}

type JIRAConfig struct {
	URL       string `toml:"url"`
	Username  string `toml:"username"`
	APIToken  string `toml:"api_token"`
	Project   string `toml:"project"`
	IssueType string `toml:"issue_type"`
}

const exampleConfig = `# example jira plugin configuration TOML file
[teleport]
auth_server = "example.com:3025"                       # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/jira/auth.key" # Teleport GRPC client secret key
client_crt = "/var/lib/teleport/plugins/jira/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/jira/auth.cas"   # Teleport cluster CA certs

[jira]
url = "https://example.com/jira"    # JIRA URL. For JIRA Cloud, https://[my-jira].atlassian.net
username = "jira-bot"               # JIRA username
api_token = "token"                 # JIRA API Basic Auth token, or our password in case you're using Jira Server.
project = "MYPROJ"                  # JIRA Project key
issue_type = "Task"                 # JIRA Issue type

[http]
public_addr = "example.com" # URL on which callback server is accessible externally, e.g. [https://]teleport-proxy.example.com
# listen_addr = ":8081" # Network address in format [addr]:port on which callback server listens, e.g. 0.0.0.0:8081
https_key_file = "/var/lib/teleport/webproxy_key.pem"  # TLS private key
https_cert_file = "/var/lib/teleport/webproxy_cert.pem" # TLS certificate

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/jira.log"
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
	if !strings.HasPrefix(c.JIRA.URL, "https://") {
		return trace.BadParameter("parameter jira.url must start with \"https://\"")
	}
	if c.JIRA.Username == "" {
		return trace.BadParameter("missing required value jira.username")
	}
	if c.JIRA.APIToken == "" {
		return trace.BadParameter("missing required value jira.api_token")
	}
	if c.JIRA.IssueType == "" {
		c.JIRA.IssueType = "Task"
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
