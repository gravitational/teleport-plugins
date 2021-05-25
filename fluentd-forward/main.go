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

	log "github.com/sirupsen/logrus"

	"github.com/gravitational/trace"
)

type dummy struct {
	A string `json:"a"`
	B string `json:"b"`
}

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

	f, err := NewFluentdClient(c)
	if err != nil {
		log.Error(trace.DebugReport(err))
		os.Exit(-1)
	}

	t, err := NewTeleportClient(c)
	if err != nil {
		log.Error(trace.DebugReport(err))
		os.Exit(-1)
	}
	defer t.Close()

	//t.Test()
	// e, err := t.Next()
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// for e != nil {
	// 	e, err := t.Next()
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	if e != nil {
	// 		fmt.Println(e.GetID())
	// 	} else {
	// 		break
	// 	}
	// }

	err = f.Send(dummy{A: "1", B: "2"})
	if err != nil {
		log.Error(trace.DebugReport(err))
		os.Exit(-1)
	}

	// err = fluentd.Init()
	// if err != nil {
	// 	log.Fatal(trace.DebugReport(err))
	// 	os.Exit(-1)
	// }

	k, err := NewCursor(c)
	if err != nil {
		log.Error(trace.DebugReport(err))
		os.Exit(-1)
	}
	k = k
}
