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

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

// TeleportSearchEventsClient is an interface for client.Client, required for testing
type TeleportSearchEventsClient interface {
	SearchEvents(ctx context.Context, fromUTC, toUTC time.Time, namespace string, eventTypes []string, limit int, startKey string) ([]events.AuditEvent, string, error)
	Close() error
}

// TeleportClient represents wrapper around Teleport client to work with events
type TeleportClient struct {
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
	batch []events.AuditEvent

	// batchSize is fetch batch size
	batchSize int

	// namespace is events namespace
	namespace string

	// types is events types list
	types []string

	// startTime is event time frame start
	startTime time.Time
}

// NewTeleportClient builds Teleport client instance
func NewTeleportClient(c *Config, cursor string, id string) (*TeleportClient, error) {
	var cl *client.Client
	var err error

	if c.TeleportIdentityFile != "" {
		cl, err = newUsingIdentityFile(c)
		if err != nil {
			return nil, err
		}
	} else {
		cl, err = newUsingKeys(c)
		if err != nil {
			return nil, err
		}
	}

	tc := TeleportClient{
		client:    cl,
		pos:       -1,
		cursor:    cursor,
		batchSize: c.BatchSize,
		namespace: c.Namespace,
		startTime: c.StartTime,
	}

	// Get the initial page and find last known event
	err = tc.fetch(id)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &tc, nil
}

// newUsingIdentityFile tries to build API client using identity file
func newUsingIdentityFile(c *Config) (*client.Client, error) {
	identity := client.LoadIdentityFile(c.TeleportIdentityFile)

	config := client.Config{
		Addrs:       []string{c.TeleportAddr},
		Credentials: []client.Credentials{identity},
	}

	client, err := client.New(context.Background(), config)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return client, nil
}

// newUsingKeys tries to build API client using keys
func newUsingKeys(c *Config) (*client.Client, error) {
	config := client.Config{
		Addrs: []string{c.TeleportAddr},
		Credentials: []client.Credentials{
			client.LoadKeyPair(c.TeleportCert, c.TeleportKey, c.TeleportCA),
		},
	}

	client, err := client.New(context.Background(), config)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return client, nil
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

	return true
}

// fetch fetches the initial page and sets the position to the event after latest known
func (t *TeleportClient) fetch(latestID string) error {
	batch, nextCursor, err := t.getEvents()
	if err != nil {
		return trace.Wrap(err)
	}

	// Save next cursor
	t.nextCursor = nextCursor

	// Mark position as unresolved (the page is empty)
	t.pos = -1

	log.WithFields(log.Fields{"cursor": t.cursor, "next": nextCursor, "len": len(batch)}).Info("Fetched page")

	// Page is empty: do nothing, return
	if len(batch) == 0 {
		return nil
	}

	pos := 0

	// If last known id is not empty, let's try to find it's pos
	if latestID != "" {
		for i, v := range batch {
			if v.GetID() == latestID {
				pos = i + 1
				t.id = latestID
			}
		}
	}

	// Set the position of the last known event
	t.pos = pos
	t.batch = batch

	log.WithFields(log.Fields{"id": t.id, "new_pos": t.pos}).Info("Skipping last known event")

	return nil
}

// getEvents calls Teleport client and loads events
func (t *TeleportClient) getEvents() ([]events.AuditEvent, string, error) {
	return t.client.SearchEvents(
		context.Background(),
		t.startTime,
		time.Now().UTC(),
		t.namespace,
		t.types,
		t.batchSize,
		t.cursor,
	)
}

// Next returns next event from a batch or requests next batch if it has been ended
func (t *TeleportClient) Next() (events.AuditEvent, string, error) {
	// The page is empty, let's re-request it to check if something has appeared
	if t.pos == -1 {
		err := t.fetch(t.id)
		if err != nil {
			return nil, t.cursor, err
		}
	}

	// We processed the last event on a page
	if t.pos >= len(t.batch) {
		// If there is the next page
		if t.flipPage() {
			// Try to flip the page
			err := t.fetch(t.id)
			if err != nil {
				return nil, t.cursor, nil
			}
		} else {
			// Try to get updates on current page
			err := t.fetch(t.id)
			if err != nil {
				return nil, t.cursor, nil
			}

			// There are no new records on current page
			if t.pos >= len(t.batch) {
				// And there is no next page, return
				if !t.flipPage() {
					return nil, t.cursor, nil
				}

				// Fetch the next page
				err = t.fetch(t.id)
				if err != nil {
					return nil, t.cursor, nil
				}
			}
		}
	}

	// After all, there is still nothing to process
	if t.pos == -1 {
		return nil, t.cursor, nil
	}

	event := t.batch[t.pos]
	t.pos++
	t.id = event.GetID()

	return event, t.cursor, nil
}
