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
	"strings"

	"github.com/gravitational/teleport/integrations/access/discord"
	"github.com/gravitational/teleport/integrations/lib"
	"github.com/gravitational/trace"
	"github.com/pelletier/go-toml"
)

// LoadDiscordConfig reads the config file, initializes a new Discord Config
// struct object, and returns it. Optionally returns an error if the file is
// not readable, or if file format is invalid.
func LoadDiscordConfig(filepath string) (*discord.Config, error) {
	t, err := toml.LoadFile(filepath)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	conf := &discord.Config{}
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
