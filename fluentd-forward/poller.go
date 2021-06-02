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
	"time"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// Poller represents periodical event poll
type Poller struct {
	// fluentd is an instance of Fluentd client
	fluentd *FluentdClient

	// teleport is an instance of Teleport client
	teleport *TeleportClient

	// state is current persisted state
	state *State

	// timeout is polling timeout
	timeout time.Duration
}

// NewPoller builds new Poller structure
func NewPoller(c *Config) (*Poller, error) {
	s, err := NewState(c)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	f, err := NewFluentdClient(c)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	cursor, err := s.GetCursor()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	id, err := s.GetID()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	log.WithFields(log.Fields{"cursor": cursor}).Info("Using initial cursor value")
	log.WithFields(log.Fields{"id": id}).Info("Using initial ID value")

	t, err := NewTeleportClient(c, cursor, id)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &Poller{fluentd: f, teleport: t, state: s, timeout: c.Timeout}, nil
}

// Close closes all connections
func (p *Poller) Close() {
	p.teleport.Close()
}

// Start starts polling
func (p *Poller) Start() error {
	g := new(errgroup.Group)
	g.Go(p.Run)

	err := g.Wait()
	if err != nil {
		return err
	}

	return nil
}

// Run is an infinite polling loop
func (p *Poller) Run() error {
	for {
		// Get next event from
		e, cursor, err := p.teleport.Next()
		if err != nil {
			return err
		}

		// No events in queue, wait and try again
		if e == nil {
			time.Sleep(p.timeout)
			continue
		}

		// Send event to fluentd
		err = p.fluentd.Send(e)
		if err != nil {
			return err
		}

		// Save latest successful id & cursor value to the state
		p.state.SetID(e.GetID())
		p.state.SetCursor(cursor)

		log.WithFields(log.Fields{"id": e.GetID(), "type": e.GetType(), "ts": e.GetTime()}).Info("Event sent")
		//log.WithFields(log.Fields{"event": e}).Debug("Event dump")
	}
}
