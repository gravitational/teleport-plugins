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
	n1, err := client.Next()

	require.NoError(t, err)
	require.IsType(t, &events.UserCreate{}, n1)

	n2, err := client.Next()

	require.NoError(t, err)
	require.IsType(t, &events.UserDelete{}, n2)

	n3, err := client.Next()

	require.NoError(t, err)
	require.Nil(t, n3)
}
