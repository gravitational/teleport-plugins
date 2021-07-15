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
	"net"
	"path"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/gravitational/trace"

	toml "github.com/pelletier/go-toml"

	"github.com/gravitational/teleport-plugins/event-handler/lib"
)

// FluentdConfig represents fluentd instance configuration
type FluentdConfig struct {
	// FluentdURL fluentd url for audit log events
	FluentdURL string `help:"fluentd url" required:"true" env:"FDFWD_FLUENTD_URL"`

	// FluentdSessionURL
	FluentdSessionURL string `help:"fluentd session url" required:"true" env:"FDFWD_FLUENTD_SESSION_URL"`

	// FluentdCert is a path to fluentd cert
	FluentdCert string `help:"fluentd TLS certificate file" required:"true" type:"existingfile" env:"FDWRD_FLUENTD_CERT"`

	// FluentdKey is a path to fluentd key
	FluentdKey string `help:"fluentd TLS key file" required:"true" type:"existingfile" env:"FDWRD_FLUENTD_KEY"`

	// FluentdCA is a path to fluentd CA
	FluentdCA string `help:"fluentd TLS CA file" type:"existingfile" env:"FDWRD_FLUENTD_CA"`
}

// TeleportConfig is Teleport instance configuration
type TeleportConfig struct {
	// TeleportAddr is a Teleport addr
	TeleportAddr string `help:"Teleport addr" env:"FDFWD_TELEPORT_ADDR"`

	// TeleportIdentityFile is a path to Teleport identity file
	TeleportIdentityFile string `help:"Teleport identity file" type:"existingfile" name:"teleport-identity" env:"FDFWD_TELEPORT_IDENTITY"`

	// TeleportCA is a path to Teleport CA file
	TeleportCA string `help:"Teleport TLS CA file" type:"existingfile" env:"FDFWD_TELEPORT_CA"`

	// TeleportCert is a path to Teleport cert file
	TeleportCert string `help:"Teleport TLS certificate file" type:"existingfile" env:"FDWRD_TELEPORT_CERT"`

	// TeleportKey is a path to Teleport key file
	TeleportKey string `help:"Teleport TLS key file" type:"existingfile" env:"FDFWD_TELEPORT_KEY"`
}

// StorageConfig represents storage config
type StorageConfig struct {
	// BaseStorageDir is a path to dv storage dir
	BaseStorageDir string `help:"Storage directory" required:"true" env:"FDFWD_STORAGE" name:"storage"`

	// StorageDir is a final storage dir prefixed with host and suffixed with dry-run
	StorageDir string `kong:"-"`
}

// IngestConfig ingestion configuration
type IngestConfig struct {
	// BatchSize is a fetch batch size
	BatchSize int `help:"Fetch batch size" default:"20" env:"FDFWD_BATCH" name:"batch"`

	// Namespace is events namespace
	Namespace string `help:"Events namespace" default:"default" env:"FDFWD_NAMESPACE"`

	// Types are event types to log
	Types []string `help:"Comma-separated list of event types to forward" env:"FDFWD_TYPES"`

	// SkipSessionTypes are session event types to skip
	SkipSessionTypesRaw []string `name:"skip-session-types" help:"Comma-separated list of session event types to skip" default:"session.print" env:"FDFWD_SKIP_SESSION_TYPES"`

	// StartTime is a time to start ingestion from
	StartTime *time.Time `help:"Minimum event time in RFC3339 format" env:"FDFWD_START_TIME"`

	// Timeout is the time poller will wait before the new request if there are no events in the queue
	Timeout time.Duration `help:"Polling timeout" default:"5s" env:"FDFWD_TIMEOUT"`

	// skipSessionTypes is a map generated from SkipSessionTypes
	SkipSessionTypes map[string]struct{} `kong:"-"`
}

// DebugConfig debug parameters
type DebugConfig struct {
	// DryRun is the flag which simulates execution without sending events to fluentd
	DryRun bool `help:"Events are read from Teleport, but are not sent to fluentd. Separate stroage is used. Debug flag."`

	// ExitOnLastEvent exit when last event is processed
	ExitOnLastEvent bool `help:"Exit when last event is processed"`
}

// ConfigureCmdConfig holds CLI options for teleport-event-handler configure
type ConfigureCmdConfig struct {
	// Out path and file prefix to put certificates into
	Out string `arg:"true" help:"Output directory" type:"existingdir" required:"true"`

	// Configure is a mock arg for now, it specifies export target
	Output string `help:"Export target service" type:"string" required:"true" default:"fluentd"`

	// Addr is Teleport auth proxy instance address
	Addr string `arg:"true" help:"Teleport auth proxy instance address" type:"string" required:"true" default:"localhost:3025"`

	// CAName CA certificate and key name
	CAName string `arg:"true" help:"CA certificate and key name" required:"true" default:"ca"`

	// ServerName server certificate and key name
	ServerName string `arg:"true" help:"Server certificate and key name" required:"true" default:"server"`

	// ClientName client certificate and key name
	ClientName string `arg:"true" help:"Client certificate and key name" required:"true" default:"client"`

	// Certificate TTL
	TTL time.Duration `help:"Certificate TTL" required:"true" default:"87600h"`

	// DNSNames is a DNS subjectAltNames for server cert
	DNSNames []string `help:"Certificate SAN hosts" default:"localhost"`

	// HostNames is an IP subjectAltNames for server cert
	IP []string `help:"Certificate SAN IPs"`

	// Length is RSA key length
	Length int `help:"Key length" enum:"1024,2048,4096" default:"2048"`

	// CN certificate common name
	CN string `help:"Common name for server cert" default:"localhost"`
}

// StartCmdConfig is start command description
type StartCmdConfig struct {
	FluentdConfig
	TeleportConfig
	StorageConfig
	IngestConfig
	DebugConfig
}

// CLI represents command structure
type CLI struct {
	// Config is the path to configuration file
	Config kong.ConfigFlag `help:"Path to TOML configuration file" optional:"true" type:"existingfile" env:"FDFWD_CONFIG"`

	// Debug is a debug logging mode flag
	Debug bool `help:"Debug logging" short:"d"`

	// Version is the version print command
	Version struct{} `cmd:"true" help:"Print plugin version"`

	// Configure is the generate certificates command configuration
	Configure ConfigureCmdConfig `cmd:"true" help:"Generate mTLS certificates for Fluentd"`

	// Start is the start command configuration
	Start StartCmdConfig `cmd:"true" help:"Start log ingestion"`
}

const (
	// fdPrefix contains section name which will be prepended with "forward."
	fdPrefix = "fluentd"

	// forwardPrefix contains prefix which must be prepended to "fluentd" section
	forwardPrefix = "forward"
)

// Validate validates start command arguments and prints them to log
func (c *StartCmdConfig) Validate() error {
	// Truncate microseconds
	if c.StartTime != nil {
		t := c.StartTime.Truncate(time.Second)
		c.StartTime = &t
	}

	d, err := c.getStorageDir()
	if err != nil {
		return trace.Wrap(err)
	}

	c.StorageDir = path.Join(c.BaseStorageDir, d)
	c.SkipSessionTypes = lib.SliceToAnonymousMap(c.SkipSessionTypesRaw)

	return nil
}

// getStorageDir returns sub dir name
func (c *StartCmdConfig) getStorageDir() (string, error) {
	host, port, err := net.SplitHostPort(c.TeleportAddr)
	if err != nil {
		return "", trace.Wrap(err)
	}

	dir := strings.TrimSpace(host + "_" + port)
	if dir == "_" {
		return "", trace.Errorf("Can not generate cursor name from Teleport host %s", c.TeleportAddr)
	}

	if c.DryRun {
		rs, err := lib.RandomString(32)
		if err != nil {
			return "", trace.Wrap(err)
		}

		dir = path.Join(dir, "dry_run", rs)
	}

	return dir, nil
}

// TOML is the kong resolver function for toml configuration file
func TOML(r io.Reader) (kong.Resolver, error) {
	config, err := toml.LoadReader(r)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// ResolverFunc reads configuration variables from the external source, TOML file in this case
	var f kong.ResolverFunc = func(context *kong.Context, parent *kong.Path, flag *kong.Flag) (interface{}, error) {
		name := flag.Name

		if strings.HasPrefix(name, fdPrefix) {
			name = strings.Join([]string{forwardPrefix, fdPrefix, name[len(fdPrefix)+1:]}, ".")
		}

		value := config.Get(name)
		valueWithinSeciton := config.Get(strings.ReplaceAll(name, "-", "."))

		if valueWithinSeciton != nil {
			return valueWithinSeciton, nil
		}

		return value, nil
	}

	return f, nil
}
