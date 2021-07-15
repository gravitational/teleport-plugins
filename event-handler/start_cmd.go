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

// import (
// 	"context"
// 	"net"
// 	"path"
// 	"strings"
// 	"time"

// 	"github.com/gravitational/teleport-plugins/event-handler/lib"
// 	"github.com/gravitational/trace"
// 	log "github.com/sirupsen/logrus"
// )

// // StartCmd is start command description
// type StartCmd struct {
// 	FluentdConfig
// 	TeleportConfig
// 	StorageConfig
// 	IngestConfig
// 	DebugConfig
// }

// // Validate validates start command arguments and prints them to log
// func (c *StartCmd) Validate() error {
// 	log.WithFields(log.Fields{"version": Version, "sha": Sha}).Printf("Teleport event handler")

// 	// Truncate microseconds
// 	if c.StartTime != nil {
// 		t := c.StartTime.Truncate(time.Second)
// 		c.StartTime = &t
// 	}

// 	// d, err := c.getStorageDir()
// 	// if err != nil {
// 	// 	return trace.Wrap(err)
// 	// }

// 	// c.StorageDir = path.Join(c.BaseStorageDir, d)

// 	// Create storage directory
// 	// _, err = os.Stat(c.StorageDir)
// 	// if os.IsNotExist(err) {
// 	// 	err = os.MkdirAll(c.StorageDir, 0755)
// 	// 	if err != nil {
// 	// 		return trace.Errorf("Can not create storage directory %v : %w", c.StorageDir, err)
// 	// 	}
// 	// }

// 	// Convert SkipSessionTypes to anonymous map (as the replacement of contains)
// 	// c.skipSessionTypes = make(map[string]struct{})

// 	// for _, v := range c.SkipSessionTypes {
// 	// 	c.skipSessionTypes[v] = struct{}{}
// 	// }

// 	// Log configuration variables
// 	log.WithField("dir", c.StorageDir).Info("Using storage dir")
// 	log.WithField("batch", c.BatchSize).Info("Using batch size")
// 	log.WithField("namespace", c.Namespace).Info("Using namespace")
// 	log.WithField("types", c.Types).Info("Using type filter")
// 	log.WithField("value", c.StartTime).Info("Using start time")
// 	log.WithField("timeout", c.Timeout).Info("Using timeout")
// 	log.WithField("url", c.FluentdURL).Info("Using Fluentd url")
// 	log.WithField("url", c.FluentdSessionURL).Info("Using Fluentd session url")
// 	log.WithField("ca", c.FluentdCA).Info("Using Fluentd ca")
// 	log.WithField("cert", c.FluentdCert).Info("Using Fluentd cert")
// 	log.WithField("key", c.FluentdKey).Info("Using Fluentd key")

// 	if c.TeleportIdentityFile != "" {
// 		log.WithField("file", c.TeleportIdentityFile).Info("Using Teleport identity file")
// 	}

// 	if c.TeleportKey != "" {
// 		log.WithField("addr", c.TeleportAddr).Info("Using Teleport addr")
// 		log.WithField("ca", c.TeleportCA).Info("Using Teleport CA")
// 		log.WithField("cert", c.TeleportCert).Info("Using Teleport cert")
// 		log.WithField("key", c.TeleportKey).Info("Using Teleport key")
// 	}

// 	if c.DryRun {
// 		log.Warn("Dry run! Events are not sent to Fluentd. Separate storage is used.")
// 	}

// 	return nil
// }

// // getStorageDir returns sub dir name
// func (c *StartCmd) getStorageDir() (string, error) {
// 	host, port, err := net.SplitHostPort(c.TeleportAddr)
// 	if err != nil {
// 		return "", trace.Wrap(err)
// 	}

// 	dir := strings.TrimSpace(host + "_" + port)
// 	if dir == "_" {
// 		return "", trace.Errorf("Can not generate cursor name from Teleport host %s", c.TeleportAddr)
// 	}

// 	if c.DryRun {
// 		rs, err := lib.RandomString(32)
// 		if err != nil {
// 			return "", trace.Wrap(err)
// 		}

// 		dir = path.Join(dir, "dry_run", rs)
// 	}

// 	return dir, nil
// }

// // Run runs the ingestion
// func (c *StartCmd) Run() error {
// 	p, err := NewPoller(context.Background(), c)
// 	if err != nil {
// 		log.Debug(trace.DebugReport(err))
// 		log.Error(err)
// 		return trace.Wrap(err)
// 	}
// 	defer p.Close()

// 	err = p.Run()
// 	if err != nil {
// 		log.Debug(trace.DebugReport(err))
// 		log.Error(err)
// 		return trace.Wrap(err)
// 	}

// 	return nil
// }
