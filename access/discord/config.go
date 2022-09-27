/*
Copyright 2022 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"github.com/gravitational/teleport-plugins/access/common"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
	"strings"
)

type DiscordConfig struct {
	common.BaseConfig
	Discord common.GenericAPIConfig
}

// LoadDiscordConfig reads the config file, initializes a new DiscordConfig struct object, and returns it.
// Optionally returns an error if the file is not readable, or if file format is invalid.
func LoadDiscordConfig(filepath string) (*DiscordConfig, error) {
	t, err := toml.LoadFile(filepath)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	conf := &DiscordConfig{}
	if err := t.Unmarshal(conf); err != nil {
		return nil, trace.Wrap(err)
	}

	if strings.HasPrefix(conf.Discord.Token, "/") {
		conf.Discord.Token, err = lib.ReadPassword(conf.Discord.Token)
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
func (c *DiscordConfig) CheckAndSetDefaults() error {
	if err := c.Teleport.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	if c.Discord.Token == "" {
		return trace.BadParameter("missing required value discord.token")
	}
	if c.Log.Output == "" {
		c.Log.Output = "stderr"
	}
	if c.Log.Severity == "" {
		c.Log.Severity = "info"
	}

	if len(c.Discord.Recipients) > 0 {
		if len(c.Recipients) > 0 {
			return trace.BadParameter("provide either discord.recipients or role_to_recipients, not both.")
		}

		c.Recipients = common.RawRecipientsMap{
			types.Wildcard: c.Discord.Recipients,
		}
	}

	if len(c.Recipients) == 0 {
		return trace.BadParameter("missing required value role_to_recipients.")
	} else if len(c.Recipients[types.Wildcard]) == 0 {
		return trace.BadParameter("missing required value role_to_recipients[%v].", types.Wildcard)
	}

	return nil
}
