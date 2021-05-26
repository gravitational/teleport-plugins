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
	"golang.org/x/net/context"
)

// TODO: -> TeleportEventsIterator
// TeleportClient represents wrapper around Teleport client to work with events
type TeleportClient struct {
	// client is an instance of GRPC Teleport client
	client *client.Client

	// cursor current cursor value
	cursor string

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
func NewTeleportClient(c *Config, cursor string) (*TeleportClient, error) {
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

	return &TeleportClient{
		client:    cl,
		pos:       -1,
		cursor:    cursor,
		batchSize: c.BatchSize,
		namespace: c.Namespace,
		startTime: c.StartTime,
	}, nil
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

// fetch fetches next batch of events starting from a last cursor position
func (t *TeleportClient) fetch() error {
	e, cursor, err := t.client.SearchEvents(
		context.Background(),
		t.startTime,
		time.Now().UTC(),
		t.namespace,
		t.types,
		t.batchSize,
		t.cursor,
	)

	if err != nil {
		return err
	}

	t.pos = -1

	for i, v := range e {
		if v.GetID() != t.cursor {
			t.pos = i
			break
		}
	}

	t.batch = e
	t.cursor = cursor

	return nil
}

// Next returns next event from a batch or requests next batch if it has been ended
func (t *TeleportClient) Next() (events.AuditEvent, error) {
	// re-request batch if it's empty or ended
	if t.pos == -1 {
		err := t.fetch()
		if err != nil {
			return nil, err
		}
	}

	// return if it's still empty
	if t.pos == -1 {
		return nil, nil
	}

	event := t.batch[t.pos]
	t.pos++

	// batch has ended
	if t.pos >= len(t.batch) {
		t.pos = -1
	}

	return event, nil
}
