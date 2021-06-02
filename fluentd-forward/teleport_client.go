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

	err = tc.fetchInitialPage(id)
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

// fetchInitialPage fetches the initial page and sets the position to the event after latest known
func (t *TeleportClient) fetchInitialPage(latestID string) error {
	log.Debug("Fetching initial event batch")

	err := t.fetch()
	if err != nil {
		return trace.Wrap(err)
	}

	t.pos = 0

	// If last known id is not empty, let's try to find it's pos
	if latestID != "" {
		for i, v := range t.batch {
			if v.GetID() == latestID {
				t.pos = i + 1
				t.id = latestID

				log.WithFields(log.Fields{"pos": t.pos, "id": latestID}).Debug("Skipping latest successful event")
			}
		}

		// Last successful event is the last event on a page, we need to flip the page
		if t.pos >= len(t.batch) {
			t.pos = -1
			return nil
		}
	} else {
		log.WithFields(log.Fields{"pos": t.pos}).Debug("No latest successful event")
	}

	return nil
}

// fetch fetches next batch of events starting from a last known cursor position
func (t *TeleportClient) fetch() error {
	batch, nextCursor, err := t.client.SearchEvents(
		context.Background(),
		t.startTime,
		time.Now().UTC(),
		t.namespace,
		t.types,
		t.batchSize,
		t.cursor,
	)

	if err != nil {
		return trace.Wrap(err)
	}

	t.nextCursor = nextCursor

	if t.cursor == nextCursor {
		log.Info("No new events loaded")
		t.pos = -1
		return nil
	}

	log.WithFields(log.Fields{"cursor": t.cursor, "nextCursor": t.nextCursor, "batchLen": len(batch)}).Info("Fetched new batch")

	// Skip latest known returned event if it came back again
	if t.id != "" {
		for i, v := range batch {
			if v.GetID() == t.id {
				t.pos = i + 1

				log.WithFields(log.Fields{"pos": t.pos, "id": t.id}).Info("Skiping latest successful event")
			}
		}

		// Flip page if latest event is last on the page
		if t.pos > len(batch) {
			t.pos = -1
		}
	}

	t.batch = batch

	return nil
}

// Next returns next event from a batch or requests next batch if it has been ended
func (t *TeleportClient) Next() (events.AuditEvent, string, error) {
	// re-request batch if it's empty or ended
	if t.pos == -1 {
		t.cursor = t.nextCursor

		err := t.fetch()
		if err != nil {
			return nil, t.cursor, err
		}

		// return if it's still empty
		if len(t.batch) == 0 || t.cursor == t.nextCursor {
			return nil, t.cursor, nil
		}
	}

	event := t.batch[t.pos]
	t.pos++

	// batch has ended
	if t.pos >= len(t.batch) {
		t.pos = -1
	}

	t.id = event.GetID()

	return event, t.cursor, nil
}
