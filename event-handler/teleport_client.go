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
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/gravitational/teleport-plugins/event-handler/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

// TeleportSearchEventsClient is an interface for client.Client, required for testing
type TeleportSearchEventsClient interface {
	SearchEvents(ctx context.Context, fromUTC, toUTC time.Time, namespace string, eventTypes []string, limit int, startKey string) ([]events.AuditEvent, string, error)
	StreamSessionEvents(ctx context.Context, sessionID string, startIndex int64) (chan events.AuditEvent, chan error)
	Close() error
}

// TeleportClient represents wrapper around Teleport client to work with events
type TeleportClient struct {
	// context is the context for a client
	context context.Context

	// client is an instance of GRPC Teleport client
	client TeleportSearchEventsClient

	// cursor current page cursor value
	cursor string

	// nextCursor next page cursor value
	nextCursor string

	// id latest event returned by Next()
	id string

	// pos current virtual cursor position within a batch
	pos int

	// batch current events batch
	batch []TeleportEvent

	// config is teleport config
	config *StartCmdConfig

	// startTime is event time frame start
	startTime time.Time

	// log is the context-specific logrus instance
	log log.FieldLogger
}

// NewTeleportClient builds Teleport client instance
func NewTeleportClient(
	ctx context.Context,
	c *StartCmdConfig,
	startTime time.Time,
	cursor string,
	id string,
) (*TeleportClient, error) {
	var err error

	config := client.Config{
		Addrs: []string{c.TeleportAddr},
		Credentials: []client.Credentials{
			client.LoadIdentityFile(c.TeleportIdentityFile),
			client.LoadKeyPair(c.TeleportCert, c.TeleportKey, c.TeleportCA),
		},
	}

	client, err := client.New(ctx, config)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	tc := TeleportClient{
		context:   ctx,
		client:    client,
		pos:       -1,
		cursor:    cursor,
		config:    c,
		startTime: startTime,
		log:       logger.Get(ctx),
	}

	return &tc, nil
}

// Close closes connection to Teleport
func (t *TeleportClient) Close() {
	t.client.Close()
}

// flipPage flips the current page
func (t *TeleportClient) flipPage() bool {
	if t.nextCursor == "" {
		return false
	}

	t.cursor = t.nextCursor
	t.pos = -1
	t.batch = []TeleportEvent{}

	return true
}

// fetch fetches the page and sets the position to the event after latest known
func (t *TeleportClient) fetch() error {
	b, nextCursor, err := t.getEvents()
	if err != nil {
		return trace.Wrap(err)
	}

	// Zero batch
	t.batch = make([]TeleportEvent, len(b))

	// Save next cursor
	t.nextCursor = nextCursor

	// Mark position as unresolved (the page is empty)
	t.pos = -1

	t.log.WithField("cursor", t.cursor).WithField("next", nextCursor).WithField("len", len(b)).Info("Fetched page")

	// Page is empty: do nothing, return
	if len(b) == 0 {
		return nil
	}

	pos := 0

	// Convert batch to TeleportEvent
	for i, e := range b {
		evt, err := NewTeleportEvent(e, t.cursor)
		if err != nil {
			return trace.Wrap(err)
		}

		t.batch[i] = evt
	}

	// If last known id is not empty, let's try to find it's pos
	if t.id != "" {
		for i, e := range t.batch {
			if e.ID == t.id {
				pos = i + 1
				t.id = e.ID
			}
		}
	}

	// Set the position of the last known event
	t.pos = pos

	t.log.WithField("id", t.id).WithField("new_pos", t.pos).Info("Skipping last known event")

	return nil
}

// getEvents calls Teleport client and loads events
func (t *TeleportClient) getEvents() ([]events.AuditEvent, string, error) {
	return t.client.SearchEvents(
		t.context,
		t.startTime,
		time.Now().UTC(),
		t.config.Namespace,
		t.config.Types,
		t.config.BatchSize,
		t.cursor,
	)
}

// pause sleeps for timeout seconds
func (t *TeleportClient) pause() {
	t.log.Infof("No new events, pause for %v seconds", t.config.Timeout)

	// Handle termination
	select {
	case <-time.After(t.config.Timeout):
		return
	case <-t.context.Done():
		return
	}
}

// getID returns event id or generates artificial id for events with blank id
func (t *TeleportClient) getID(event events.AuditEvent) (string, error) {
	id := event.GetID()
	if id != "" {
		return id, nil
	}

	data, err := lib.FastMarshal(event)
	if err != nil {
		return "", trace.Wrap(err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// Next returns next event from a batch or requests next batch if it has been ended
func (t *TeleportClient) Events() (chan *TeleportEvent, chan error) {
	ch := make(chan *TeleportEvent)
	e := make(chan error, 1)

	go func() {
		defer close(ch)
		defer close(e)

		for {
			// If there is nothing in the batch, request
			if len(t.batch) == 0 {
				err := t.fetch()
				if err != nil {
					e <- trace.Wrap(err)
					break
				}

				// If there is still nothing, sleep
				if len(t.batch) == 0 {
					if t.config.ExitOnLastEvent {
						log.Info("All events are processed, exiting...")
						break
					}

					t.pause()
					continue
				}
			}

			// If we processed the last event on a page
			if t.pos >= len(t.batch) {
				// If there is next page, flip page
				if t.flipPage() {
					continue
				}

				// If not, update current page
				err := t.fetch()
				if err != nil {
					e <- trace.Wrap(err)
					continue
				}

				// If there is still nothing new on current page, sleep
				if t.pos >= len(t.batch) {
					if t.config.ExitOnLastEvent {
						log.Info("All events are processed, exiting...")
						break
					}

					t.pause()
					continue
				}
			}

			event := t.batch[t.pos]
			t.pos++
			t.id = event.ID

			ch <- &event
		}
	}()

	return ch, e
}

// StreamSessionEvents returns session event stream, that's the simple delegate to an API function
func (t *TeleportClient) StreamSessionEvents(ctx context.Context, id string, index int64) (chan events.AuditEvent, chan error) {
	return t.client.StreamSessionEvents(ctx, id, index)
}
