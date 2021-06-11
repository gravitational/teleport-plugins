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

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

func init() {
	initConfig()
}

func main() {
	log.WithFields(log.Fields{"version": Version, "sha": Sha}).Printf("Teleport fluentd-forwarder")

	c, err := newConfig()
	if err != nil {
		log.Error(trace.DebugReport(err))
		printUsage()
		os.Exit(-1)
	}

	p, err := NewPoller(c)
	if err != nil {
		log.Debug(trace.DebugReport(err))
		log.Error(err)
		os.Exit(-1)
	}
	defer p.Close()

	err = p.Run()
	if err != nil {
		log.Debug(trace.DebugReport(err))
		log.Error(err)
		os.Exit(-1)
	}
}
