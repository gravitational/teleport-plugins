package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"os"

	"github.com/pelletier/go-toml"

	"github.com/gravitational/trace"
)

type Config struct {
	AuthServer string `toml:"auth-server"`
	ClientKey  string `toml:"client-key"`
	ClientCrt  string `toml:"client-crt"`
	RootCAs    string `toml:"root-cas"`
}

const exampleConfig = `# example configuration file
auth-server = "example.com:3025"  # Auth GRPC API address
client-key = "path/to/client.key" #  GRPC client secret key
client-crt = "path/to/client.crt" #  GRPC client certificate 
root-cas = "path/to/root.cas"     #  cluster CA certs
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
	conf.SetDefaults()
	return conf, nil
}

func (c *Config) SetDefaults() {
	if c.AuthServer == "" {
		c.AuthServer = "localhost:3025"
	}
	if c.ClientKey == "" {
		c.ClientKey = "example.key"
	}
	if c.ClientCrt == "" {
		c.ClientCrt = "example.crt"
	}
	if c.RootCAs == "" {
		c.RootCAs = "example.cas"
	}
}

// LoadTLSConfig loads client crt/key files and root authorities, and
// generates a tls.Config suitable for use with a GRPC client.
func (c *Config) LoadTLSConfig() (*tls.Config, error) {
	var tc tls.Config
	clientCert, err := tls.LoadX509KeyPair(c.ClientCrt, c.ClientKey)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	tc.Certificates = append(tc.Certificates, clientCert)
	caFile, err := os.Open(c.RootCAs)
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
