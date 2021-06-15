/*
Copyright 2015-2021 Gravitational, Inc.

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
	"io"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"

	toml "github.com/pelletier/go-toml"
)

// CLI represents command structure
type CLI struct {
	// Config is the path to configuration file
	Config kong.ConfigFlag `help:"Path to TOML configuration file" optional:"true" type:"existingfile" env:"FDFWD_CONFIG"`

	// Debug is a debug logging mode flag
	Debug bool `help:"Debug logging" short:"d"`

	// Start is the start command configuration
	Start StartCmd `cmd:"true" help:"Start log ingestion"`

	// GenCerts is the generate certificates command configuration
	GenCerts GenCertsCmd `cmd:"true" help:"Generate mTLS certificates for Fluentd"`
}

// Validate validates base CLI structure, switches on debug logging if needed
func (c *CLI) Validate() error {
	if c.Debug {
		log.SetLevel(log.DebugLevel)
	}

	return nil
}

// TOML is the kong resolver function for toml configuration file
func TOML(r io.Reader) (kong.Resolver, error) {
	config, err := toml.LoadReader(r)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// ResolverFunc reads configuration variables from the external source, TOML file in this case
	var f kong.ResolverFunc = func(context *kong.Context, parent *kong.Path, flag *kong.Flag) (interface{}, error) {
		value := config.Get(flag.Name)
		valueWithinSeciton := config.Get(strings.ReplaceAll(flag.Name, "-", "."))

		if valueWithinSeciton != nil {
			return valueWithinSeciton, nil
		}

		return value, nil
	}

	return f, nil
}
