package main

import (
	"strings"

	"github.com/gravitational/teleport-plugins/access/config"
	"github.com/gravitational/teleport-plugins/access/ms-teams/msapi"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

// Config represents plugin configuration
type Config struct {
	Teleport   lib.TeleportConfig
	Recipients config.RecipientsMap `toml:"role_to_recipients"`
	Log        logger.Config
	MSAPI      msapi.Config `toml:"msapi"`
	Preload    bool         `toml:"preload"`
}

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

	// Azure secret format does not seem to support starting with a "/"
	if strings.HasPrefix(conf.MSAPI.AppSecret, "/") {
		conf.MSAPI.AppSecret, err = lib.ReadPassword(conf.MSAPI.AppSecret)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}

	err = conf.CheckAndSetDefaults()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return conf, nil
}

// CheckAndSetDefaults checks the config struct for any logical errors, and sets default values
// if some values are missing.
// If critical values are missing and we can't set defaults for them — this will return an error.
func (c *Config) CheckAndSetDefaults() error {
	if err := c.Teleport.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}

	if c.Log.Output == "" {
		c.Log.Output = "stderr"
	}
	if c.Log.Severity == "" {
		c.Log.Severity = "info"
	}

	if len(c.Recipients) == 0 {
		return trace.BadParameter("missing required value role_to_recipients.")
	} else if len(c.Recipients[types.Wildcard]) == 0 {
		return trace.BadParameter("missing required value role_to_recipients[%v].", types.Wildcard)
	}

	return nil
}
