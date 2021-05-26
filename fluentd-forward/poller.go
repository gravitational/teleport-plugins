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
	"github.com/gravitational/trace"
)

// Poller represents periodical event poll
type Poller struct {
	// fluentd is an instance of Fluentd client
	fluentd *FluentdClient

	// teleport is an instance of Teleport client
	teleport *TeleportClient

	// cursor is an instance of cursor manager
	cursor *Cursor
}

// NewPoller builds new Poller structure
func NewPoller(c *Config) (*Poller, error) {
	k, err := NewCursor(c)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	f, err := NewFluentdClient(c)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	cursor, err := k.Get()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	t, err := NewTeleportClient(c, cursor)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &Poller{fluentd: f, teleport: t, cursor: k}, nil
}

// Close closes all connections
func (p *Poller) Close() {
	p.teleport.Close()
}

func (p *Poller) Run() {
	// // v, _ := k.Get()
	// // logrus.Printf(v)
	// // k.Set("")

	// err = f.Send(dummy{A: "1", B: "2"})
	// if err != nil {
	// 	log.Error(trace.DebugReport(err))
	// 	os.Exit(-1)
	// }

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

	// err = fluentd.Init()
	// if err != nil {
	// 	log.Fatal(trace.DebugReport(err))
	// 	os.Exit(-1)
	// }

}
