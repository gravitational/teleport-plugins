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
	"context"
	"time"

	"github.com/gravitational/teleport/api/types/events"
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

	// cmd is a reference to StartCmd with configuration options
	cmd *StartCmdConfig

	// context is the poller context
	context context.Context

	// eg is an errgroup for all session routines
	eg *errgroup.Group
}

const (
	// sessionEndType type name for session end event
	sessionEndType = "session.end"
)

// NewPoller builds new Poller structure
func NewPoller(ctx context.Context, c *StartCmdConfig) (*Poller, error) {
	// s, err := NewState(&c.StorageConfig, &c.IngestConfig)
	// if err != nil {
	// 	return nil, trace.Wrap(err)
	// }

	// f, err := NewFluentdClient(c.FluentdURL, &c.FluentdConfig)
	// if err != nil {
	// 	return nil, trace.Wrap(err)
	// }

	// cursor, err := s.GetCursor()
	// if err != nil {
	// 	return nil, trace.Wrap(err)
	// }

	// id, err := s.GetID()
	// if err != nil {
	// 	return nil, trace.Wrap(err)
	// }

	// st, err := s.GetStartTime()
	// if err != nil {
	// 	return nil, trace.Wrap(err)
	// }

	// log.WithField("cursor", cursor).Info("Using initial cursor value")
	// log.WithField("id", id).Info("Using initial ID value")
	// log.WithField("value", st).Info("Using start time from state")

	// eg, egCtx := errgroup.WithContext(ctx)

	// t, err := NewTeleportClient(ctx, &c.TeleportConfig, &c.IngestConfig, *st, cursor, id)
	// if err != nil {
	// 	return nil, trace.Wrap(err)
	// }

	// return &Poller{
	// 	context:  egCtx,
	// 	fluentd:  f,
	// 	teleport: t,
	// 	state:    s,
	// 	cmd:      c,
	// 	eg:       eg,
	// }, nil

	return nil, nil
}

// Close closes all connections
func (p *Poller) Close() {
	p.teleport.Close()
}

func (p *Poller) sendEvent(c *FluentdClient, e events.AuditEvent) error {
	// Send event to fluentd if it is not a dry run
	if !p.cmd.DryRun {
		err := c.Send(e)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	log.WithFields(log.Fields{"id": e.GetID(), "type": e.GetType(), "ts": e.GetTime(), "index": e.GetIndex()}).Info("Event sent")
	log.WithField("event", e).Debug("Event dump")

	return nil
}

// startPollSessionOnSessionEnd starts session poll based on session.end event
func (p *Poller) startPollSessionOnSessionEnd(e events.AuditEvent) error {
	evt := events.MustToOneOf(e)

	id := evt.GetSessionEnd().SessionID

	log.WithField("id", id).Info("Started session events ingest")

	return p.pollSession(id, 0)
}

// pollSession polls session events
func (p *Poller) pollSession(id string, index int) error {
	log.WithField("id", id).Info("Started session events ingest")

	fluentd, err := NewFluentdClient(p.cmd.FluentdSessionURL+"."+id+".log", &p.cmd.FluentdConfig)
	if err != nil {
		return trace.Wrap(err)
	}

	chEvt, chErr := p.teleport.StreamSessionEvents(p.context, id, int64(index))

	err = p.state.SetSessionIndex(id, 0)
	if err != nil {
		return trace.Wrap(err)
	}

	for {
		select {
		case err := <-chErr:
			return trace.Wrap(err)

		case evt := <-chEvt:
			if evt == nil {
				log.WithField("id", id).Info("Finished session events ingest")

				// Session export has finished, we do not need it's state anymore
				err := p.state.RemoveSession(id)
				if err != nil {
					return trace.Wrap(err)
				}

				return nil
			}

			_, ok := p.cmd.SkipSessionTypes[evt.GetType()]
			if !ok {
				err = p.sendEvent(fluentd, evt)
				if err != nil {
					return trace.Wrap(err)
				}
			}

			// Set session index
			err = p.state.SetSessionIndex(id, evt.GetIndex())
			if err != nil {
				return trace.Wrap(err)
			}
		}
	}
}

// pollAuditLog polls an audit log
func (p *Poller) pollAuditLog() error {
	for {
		// Get next event from
		e, cursor, err := p.teleport.Next()
		if err != nil {
			return trace.Wrap(err)
		}

		// No events in queue, wait and try again
		if e == nil {
			if p.cmd.ExitOnLastEvent {
				log.Info("All events have been processed! Exiting...")
				return nil
			}

			log.WithField("timeout", p.cmd.Timeout).Debug("Idling")
			time.Sleep(p.cmd.Timeout)
			continue
		}

		err = p.sendEvent(p.fluentd, e)
		if err != nil {
			return trace.Wrap(err)
		}

		// Start session export
		if e.GetType() == sessionEndType {
			p.eg.Go(func() error {
				return p.startPollSessionOnSessionEnd(e)
			})
		}

		// Save latest successful id & cursor value to the state
		p.state.SetID(e.GetID())
		p.state.SetCursor(cursor)
	}
}

// Run polling loop
func (p *Poller) Run() error {
	s, err := p.state.GetSessions()
	if err != nil {
		return trace.Wrap(err)
	}

	if len(s) > 0 {
		for id, idx := range s {
			func(id string, idx int) {
				log.WithFields(log.Fields{"id": id, "index": idx}).Info("Restarting session ingestion")

				p.eg.Go(func() error {
					return p.pollSession(id, idx)
				})
			}(id, int(idx)) // That's weird that index is not int64 while it is in the event itself
		}
	}

	p.eg.Go(p.pollAuditLog)

	err = p.eg.Wait()
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}
