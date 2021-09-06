package lib

import (
	"strings"

	"github.com/gravitational/teleport-plugins/lib/stringset"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
)

// TeleportConfig stores config options for where
// the Teleport's Auth server is listening, and what certificates to
// use to authenticate in it.
type TeleportConfig struct {
	AuthServer string `toml:"auth_server"`
	Addr       string `toml:"addr"`
	Identity   string `toml:"identity"`
	ClientKey  string `toml:"client_key"`
	ClientCrt  string `toml:"client_crt"`
	RootCAs    string `toml:"root_cas"`
}

func (cfg TeleportConfig) GetAddrs() []string {
	if cfg.Addr != "" {
		return []string{cfg.Addr}
	} else if cfg.AuthServer != "" {
		return []string{cfg.AuthServer}
	}
	return nil
}

func (cfg *TeleportConfig) CheckAndSetDefaults() error {
	if cfg.Addr == "" && cfg.AuthServer == "" {
		cfg.Addr = "localhost:3025"
	} else if cfg.AuthServer != "" {
		log.Warn("Configuration setting `auth_server` is deprecated, consider to change it to `addr`")
	}

	if err := cfg.CheckTLSConfig(); err != nil {
		return trace.Wrap(err)
	}

	if cfg.Identity != "" && cfg.ClientCrt != "" {
		return trace.BadParameter("configuration setting `identity` is mutually exclusive with all the `client_crt`, `client_key` and `root_cas` settings")
	}

	return nil
}

func (cfg *TeleportConfig) CheckTLSConfig() error {
	provided := stringset.NewWithCap(3)
	missing := stringset.NewWithCap(3)

	if cfg.ClientCrt != "" {
		provided.Add("`client_crt`")
	} else {
		missing.Add("`client_crt`")
	}

	if cfg.ClientKey != "" {
		provided.Add("`client_key`")
	} else {
		missing.Add("`client_key`")
	}

	if cfg.RootCAs != "" {
		provided.Add("`root_cas`")
	} else {
		missing.Add("`root_cas`")
	}

	if len(provided) > 0 && len(provided) < 3 {
		return trace.BadParameter(
			"configuration setting(s) %s are provided but setting(s) %s are missing",
			strings.Join(provided.ToSlice(), ", "),
			strings.Join(missing.ToSlice(), ", "),
		)
	}

	return nil
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
