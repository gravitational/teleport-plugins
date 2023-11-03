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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"

	"github.com/gravitational/teleport/api/client/proto"
	auditlogpb "github.com/gravitational/teleport/api/gen/proto/go/teleport/auditlog/v1"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/events"
)

// mockTeleportEventWatcher is Teleport client mock
type mockTeleportEventWatcher struct {
	// events is the mock list of events
	events []events.AuditEvent
}

func (c *mockTeleportEventWatcher) SearchEvents(ctx context.Context, fromUTC, toUTC time.Time, namespace string, eventTypes []string, limit int, order types.EventOrder, startKey string) ([]events.AuditEvent, string, error) {
	e := c.events
	c.events = nil
	return e, "test", nil
}

func (c *mockTeleportEventWatcher) StreamSessionEvents(ctx context.Context, sessionID string, startIndex int64) (chan events.AuditEvent, chan error) {
	return nil, nil
}

func (c *mockTeleportEventWatcher) SearchUnstructuredEvents(ctx context.Context, fromUTC, toUTC time.Time, namespace string, eventTypes []string, limit int, order types.EventOrder, startKey string) ([]*auditlogpb.EventUnstructured, string, error) {
	e := c.events
	c.events = nil

	protoEvents, err := eventsToProto(e)
	if err != nil {
		return nil, "", trace.Wrap(err)
	}

	return protoEvents, "test", nil
}

func (c *mockTeleportEventWatcher) StreamUnstructuredSessionEvents(ctx context.Context, sessionID string, startIndex int64) (chan *auditlogpb.EventUnstructured, chan error) {
	return nil, nil
}

func (c *mockTeleportEventWatcher) UpsertLock(ctx context.Context, lock types.Lock) error {
	return nil
}

func (c *mockTeleportEventWatcher) Ping(ctx context.Context) (proto.PingResponse, error) {
	return proto.PingResponse{
		ServerVersion: Version,
	}, nil
}

func (c *mockTeleportEventWatcher) Close() error {
	return nil
}

func newTeleportEventWatcher(t *testing.T, eventsClient TeleportSearchEventsClient, exitOnLastEvent bool) *TeleportEventsWatcher {
	client := &TeleportEventsWatcher{
		client: eventsClient,
		pos:    -1,
		config: &StartCmdConfig{
			IngestConfig: IngestConfig{
				BatchSize:       5,
				ExitOnLastEvent: exitOnLastEvent,
			},
		},
	}

	return client
}

func TestNext(t *testing.T) {
	const mockEventID = "1"
	e := []events.AuditEvent{
		&events.UserCreate{
			Metadata: events.Metadata{
				ID: mockEventID,
			},
		},
		&events.UserDelete{
			Metadata: events.Metadata{
				ID: mockEventID,
			},
		},
	}

	mockEventWatcher := &mockTeleportEventWatcher{e}
	client := newTeleportEventWatcher(t, mockEventWatcher, true)
	chEvt, chErr := client.Events(context.Background())

	select {
	case err := <-chErr:
		t.Fatalf("received unexpected error from error channel: %v", err)
	case e := <-chEvt:
		require.NotNil(t, e.Event)
		require.Equal(t, mockEventID, e.ID)
	case <-time.After(time.Second):
		t.Fatalf("No events received withing one second")
	}

	select {
	case err := <-chErr:
		t.Fatalf("received unexpected error from error channel: %v", err)
	case e := <-chEvt:
		require.NotNil(t, e.Event)
		require.Equal(t, mockEventID, e.ID)
	case <-time.After(time.Second):
		t.Fatalf("No events received withing one second")
	}
}

// errMockTeleportEventWatcher is Teleport client mock that returns an error after the first SearchUnstructuredEvents
type errMockTeleportEventWatcher struct {
	mockTeleportEventWatcher
	searchUnstructuredEventsCalled bool
}

func (c *errMockTeleportEventWatcher) SearchUnstructuredEvents(ctx context.Context, fromUTC, toUTC time.Time, namespace string, eventTypes []string, limit int, order types.EventOrder, startKey string) ([]*auditlogpb.EventUnstructured, string, error) {
	if c.searchUnstructuredEventsCalled {
		return nil, "", errors.New("error")
	}
	defer func() { c.searchUnstructuredEventsCalled = true }()

	return c.mockTeleportEventWatcher.SearchUnstructuredEvents(ctx, fromUTC, toUTC, namespace, eventTypes, limit, order, startKey)
}

func TestLastEvent(t *testing.T) {
	t.Run("should not leave hanging go-routines", func(t *testing.T) {
		const mockEventID = "1"
		e := []events.AuditEvent{
			&events.UserCreate{
				Metadata: events.Metadata{
					ID: mockEventID,
				},
			},
		}

		mockEventWatcher := &errMockTeleportEventWatcher{mockTeleportEventWatcher: mockTeleportEventWatcher{e}}
		client := newTeleportEventWatcher(t, mockEventWatcher, true)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.Cleanup(cancel)

		chEvt, chErr := client.Events(ctx)

		select {
		case err := <-chErr:
			t.Fatalf("received unexpected error from error channel: %v", err)
		case e := <-chEvt:
			require.NotNil(t, e.Event)
			require.Equal(t, mockEventID, e.ID)
		case <-time.After(time.Second):
			t.Fatalf("No events received withing one second")
		}

		var wg sync.WaitGroup

		const numGoroutines = 5
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				chEvt, _ := client.Events(ctx)
				// consume events.
				for range chEvt {
				}
			}()
		}

		goroutinesDone := make(chan struct{})
		go func() {
			wg.Wait()
			close(goroutinesDone)
		}()

		select {
		case <-goroutinesDone:
		case <-ctx.Done():
			require.Fail(t, "timeout reached, some goroutines were not closed")
		}
	})
}

func TestValidateConfig(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cfg       StartCmdConfig
		wantError bool
	}{
		{
			name: "Identity file configured",
			cfg: StartCmdConfig{
				FluentdConfig{},
				TeleportConfig{
					TeleportIdentityFile: "not_empty_string",
				},
				IngestConfig{},
				LockConfig{},
			},
			wantError: false,
		}, {
			name: "Cert, key, ca files configured",
			cfg: StartCmdConfig{
				FluentdConfig{},
				TeleportConfig{
					TeleportCA:   "not_empty_string",
					TeleportCert: "not_empty_string",
					TeleportKey:  "not_empty_string",
				},
				IngestConfig{},
				LockConfig{},
			},
			wantError: false,
		}, {
			name: "Identity and teleport cert/ca/key files configured",
			cfg: StartCmdConfig{
				FluentdConfig{},
				TeleportConfig{
					TeleportIdentityFile: "not_empty_string",
					TeleportCA:           "not_empty_string",
					TeleportCert:         "not_empty_string",
					TeleportKey:          "not_empty_string",
				},
				IngestConfig{},
				LockConfig{},
			},
			wantError: true,
		}, {
			name: "None set",
			cfg: StartCmdConfig{
				FluentdConfig{},
				TeleportConfig{},
				IngestConfig{},
				LockConfig{},
			},
			wantError: true,
		}, {
			name: "Some of teleport cert/key/ca unset",
			cfg: StartCmdConfig{
				FluentdConfig{},
				TeleportConfig{
					TeleportCA: "not_empty_string",
				},
				IngestConfig{},
				LockConfig{},
			},
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantError {
				require.True(t, trace.IsBadParameter(err))
				return
			}
			require.NoError(t, err)
		})
	}
}
