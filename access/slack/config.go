package main

import (
	"fmt"
	"strings"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

// Config stores the full configuration for the teleport-slack plugin to run.
type Config struct {
	Teleport   lib.TeleportConfig
	Slack      SlackConfig
	Recipients RecipientsMap `toml:"role_to_recipients"`
	Log        logger.Config
}

// SlackConfig holds Slack-specific configuration options.
type SlackConfig struct {
	Token string
	// DELETE IN 10.0.0 (Joerger) - use "role_to_recipients["*"]" instead
	Recipients []string
	APIURL     string
}

// RecipientsMap is a mapping of roles to recipient(s).
type RecipientsMap map[string][]string

func (r *RecipientsMap) UnmarshalTOML(in interface{}) error {
	*r = make(RecipientsMap)

	recipientsMap, ok := in.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected type for recipients %T", in)
	}

	for k, v := range recipientsMap {
		switch val := v.(type) {
		case string:
			(*r)[k] = []string{val}
		case []interface{}:
			for _, str := range val {
				str, ok := str.(string)
				if !ok {
					return fmt.Errorf("unexpected type for recipients value %T", v)
				}
				(*r)[k] = append((*r)[k], str)
			}
		default:
			return fmt.Errorf("unexpected type for recipients value %T", v)
		}
	}

	return nil
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

	if strings.HasPrefix(conf.Slack.Token, "/") {
		conf.Slack.Token, err = lib.ReadPassword(conf.Slack.Token)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}

	if err := conf.CheckAndSetDefaults(); err != nil {
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
	if c.Slack.Token == "" {
		return trace.BadParameter("missing required value slack.token")
	}
	if c.Log.Output == "" {
		c.Log.Output = "stderr"
	}
	if c.Log.Severity == "" {
		c.Log.Severity = "info"
	}

	if c.Slack.Recipients != nil {
		if c.Recipients != nil {
			return trace.BadParameter("provide either slack.recipients or role_to_recipients, not both.")
		}

		c.Recipients = RecipientsMap{
			types.Wildcard: c.Slack.Recipients,
		}
	}

	if c.Recipients == nil {
		return trace.BadParameter("missing required value role_to_recipients.")
	} else if c.Recipients[types.Wildcard] == nil {
		return trace.BadParameter("missing required value role_to_recipients[%q].", types.Wildcard)
	}

	return nil
}
