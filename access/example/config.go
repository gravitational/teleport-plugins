/*
Copyright 2019-2021 Gravitational, Inc.

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
	"github.com/pelletier/go-toml"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/trace"
)

type Config struct {
	lib.TeleportConfig
	Whitelist []string `toml:"whitelist"`
}

const exampleConfig = `# example configuration file
# Teleport Auth/Proxy Server address.
addr = "localhost:3025"
# Identity file exported with tctl auth sign --format=file
identity = "/var/lib/teleport/plugins/gitlab/auth_id"
# whitelist determines which users' requests will
# be approved.
whitelist = [
    "alice",
    "bob",
]
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
