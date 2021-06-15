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
	"os"
	"time"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

type StartCmd struct {
	FluentdURL string `help:"fluentd url" required:"true" env:"FDFWD_FLUENTD_URL"`

	// FluentdCert is a path to fluentd cert
	FluentdCert string `help:"fluentd TLS certificate file" required:"true" type:"existingfile" env:"FDWRD_FLUENTD_CERT"`

	// FluentdKey is a path to fluentd key
	FluentdKey string `help:"fluentd TLS key file" required:"true" type:"existingfile" env:"FDWRD_FLUENTD_KEY"`

	// FluentdCA is a path to fluentd CA
	FluentdCA string `help:"fluentd TLS CA file" type:"existingfile" env:"FDWRD_FLUENTD_CA"`

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

	// StorageDir is a path to badger storage dir
	StorageDir string `help:"Storage directory" required:"true" env:"FDFWD_STORAGE" name:"storage"`

	// BatchSize is a fetch batch size
	BatchSize int `help:"Fetch batch size" default:"20" env:"FDFWD_BATCH" name:"batch"`

	// Namespace is events namespace
	Namespace string `help:"Events namespace" default:"default" env:"FDFWD_NAMESPACE"`

	// Types are event types to log
	Types []string `help:"Comma-separated list of event types to forward" env:"FDFWD_TYPES"`

	// StartTime is a time to start ingestion from
	StartTime *time.Time `help:"Minimum event time in RFC3339 format" env:"FDFWD_START_TIME"`

	// Timeout is the time poller will wait before the new request if there are no events in the queue
	Timeout time.Duration `help:"Polling timeout" default:"5s" env:"FDFWD_TIMEOUT"`
}

// Validate validates start command arguments and prints them to log
func (c *StartCmd) Validate() error {
	// Truncate microseconds
	if c.StartTime != nil {
		t := c.StartTime.Truncate(time.Second)
		c.StartTime = &t
	}

	// Create storage directory
	_, err := os.Stat(c.StorageDir)
	if os.IsNotExist(err) {
		err = os.MkdirAll(c.StorageDir, 0755)
		if err != nil {
			return trace.Errorf("Can not create storage directory %v : %w", c.StorageDir, err)
		}
	}

	// Log configuration variables
	log.WithField("dir", c.StorageDir).Info("Using storage dir")
	log.WithField("batch", c.BatchSize).Info("Using batch size")
	log.WithField("namespace", c.Namespace).Info("Using namespace")
	log.WithField("types", c.Types).Info("Using type filter")
	log.WithField("value", c.StartTime).Info("Using start time")
	log.WithField("timeout", c.Timeout).Info("Using timeout")
	log.WithField("url", c.FluentdURL).Info("Using Fluentd url")
	log.WithField("ca", c.FluentdCA).Info("Using Fluentd ca")
	log.WithField("cert", c.FluentdCert).Info("Using Fluentd cert")
	log.WithField("key", c.FluentdKey).Info("Using Fluentd key")

	if c.TeleportIdentityFile != "" {
		log.WithField("file", c.TeleportIdentityFile).Info("Using Teleport identity file")
	}

	if c.TeleportKey != "" {
		log.WithField("addr", c.TeleportAddr).Info("Using Teleport addr")
		log.WithField("ca", c.TeleportCA).Info("Using Teleport CA")
		log.WithField("cert", c.TeleportCert).Info("Using Teleport cert")
		log.WithField("key", c.TeleportKey).Info("Using Teleport key")
	}

	return nil
}

// Run runs the ingestion
func (c *StartCmd) Run() error {
	p, err := NewPoller(c)
	if err != nil {
		log.Debug(trace.DebugReport(err))
		log.Error(err)
		return err
	}
	defer p.Close()

	err = p.Run()
	if err != nil {
		log.Debug(trace.DebugReport(err))
		log.Error(err)
		return err
	}

	return nil
}
