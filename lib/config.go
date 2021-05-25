package lib

import "github.com/gravitational/teleport/api/client"

// TeleportConfig stores config options for where
// the Teleport's Auth server is listening, and what certificates to
// use to authenticate in it.
type TeleportConfig struct {
	AuthServer string `toml:"auth_server"`
	Identity   string `toml:"identity"`
	ClientKey  string `toml:"client_key"`
	ClientCrt  string `toml:"client_crt"`
	RootCAs    string `toml:"root_cas"`
}

func (cfg TeleportConfig) Credentials() []client.Credentials {
	switch true {
	case cfg.Identity != "":
		return []client.Credentials{client.LoadIdentityFile(cfg.Identity)}
	case cfg.ClientCrt != "" && cfg.ClientKey != "" && cfg.RootCAs != "":
		return []client.Credentials{client.LoadKeyPair(cfg.ClientCrt, cfg.ClientKey, cfg.RootCAs)}
	default:
		return nil
	}
}
