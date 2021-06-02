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
	"testing"
	"time"

	"github.com/gravitational/teleport/api/types/events"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

// mockTeleportClient is Teleport client mock
type mockTeleportClient struct {
	// events is the mock list of events
	events []events.AuditEvent
}

// SearchEvents is mock SearchEvents method which returns events
func (c *mockTeleportClient) SearchEvents(ctx context.Context, fromUTC, toUTC time.Time, namespace string, eventTypes []string, limit int, startKey string) ([]events.AuditEvent, string, error) {
	e := c.events
	c.events = make([]events.AuditEvent, 0) // nullify events
	return e, "test", nil
}

// Close is mock close method
func (c *mockTeleportClient) Close() error {
	return nil
}

func newTeleportClient(e []events.AuditEvent) *TeleportClient {
	teleportClient := &mockTeleportClient{events: e}

	client := &TeleportClient{
		client:    teleportClient,
		pos:       -1,
		batchSize: 5,
	}

	return client
}

func TestNext(t *testing.T) {
	e := make([]events.AuditEvent, 2)
	e[0] = &events.UserCreate{
		Metadata: events.Metadata{
			ID: "1",
		},
	}
	e[1] = &events.UserDelete{
		Metadata: events.Metadata{
			ID: "2",
		},
	}

	client := newTeleportClient(e)
	n1, _, err := client.Next()

	require.NoError(t, err)
	require.IsType(t, &events.UserCreate{}, n1)

	n2, _, err := client.Next()

	require.NoError(t, err)
	require.IsType(t, &events.UserDelete{}, n2)

	n3, _, err := client.Next()

	require.NoError(t, err)
	require.Nil(t, n3)
}
