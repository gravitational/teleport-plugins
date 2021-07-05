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
	cmd *StartCmd

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
func NewPoller(ctx context.Context, c *StartCmd) (*Poller, error) {
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

	st, err := s.GetStartTime()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	log.WithField("cursor", cursor).Info("Using initial cursor value")
	log.WithField("id", id).Info("Using initial ID value")
	log.WithField("value", st).Info("Using start time from state")

	eg, egCtx := errgroup.WithContext(ctx)

	t, err := NewTeleportClient(ctx, c, *st, cursor, id)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &Poller{
		context:  egCtx,
		fluentd:  f,
		teleport: t,
		state:    s,
		cmd:      c,
		eg:       eg,
	}, nil
}

// Close closes all connections
func (p *Poller) Close() {
	p.teleport.Close()
}

// pollSession polls session events
func (p *Poller) pollSession(e events.AuditEvent) error {
	evt, err := events.ToOneOf(e)
	if err != nil {
		return trace.Wrap(err)
	}

	id := evt.GetSessionEnd().SessionID

	log.WithField("id", id).Info("Start session events ingest")

	chanEvt, chanErr := p.teleport.StreamSessionEvents(p.context, id, 0)

	for {
		select {
		case evt := <-chanEvt:
			log.WithField("evt", evt).Info("Session event read")

		case err := <-chanErr:
			return err

		default:
			log.WithField("id", id).Info("Session events read")
			return nil
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

		// Send event to fluentd if it is not a dry run
		if !p.cmd.DryRun {
			err = p.fluentd.Send(e)
			if err != nil {
				return trace.Wrap(err)
			}
		}

		// Start session export
		if e.GetType() == sessionEndType {
			p.eg.Go(func() error {
				return p.pollSession(e)
			})
		}

		// Save latest successful id & cursor value to the state
		p.state.SetID(e.GetID())
		p.state.SetCursor(cursor)

		log.WithFields(log.Fields{"id": e.GetID(), "type": e.GetType(), "ts": e.GetTime()}).Info("Event sent")
		log.WithField("event", e).Debug("Event dump")
	}
}

// Run polling loop
func (p *Poller) Run() error {
	p.eg.Go(p.pollAuditLog)

	err := p.eg.Wait()
	if err != nil {
		return err
	}

	return nil
}
