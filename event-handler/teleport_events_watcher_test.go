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
	"testing"
	"time"

	"github.com/gravitational/teleport/api/client/proto"
	auditlogpb "github.com/gravitational/teleport/api/gen/proto/go/teleport/auditlog/v1"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/trace"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

// mockTeleportEventWatcher is Teleport client mock
type mockTeleportEventWatcher struct {
	// events is the mock list of events
	events []events.AuditEvent
	t      *testing.T
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

	events := make([]*auditlogpb.EventUnstructured, len(e))
	for i, event := range e {
		events[i] = eventToJSON(c.t, event)
	}
	return events, "test", nil
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

func newTeleportEventWatcher(t *testing.T, e []events.AuditEvent, exitOnLastEvent bool) *TeleportEventsWatcher {
	teleportEventWatcher := &mockTeleportEventWatcher{events: e, t: t}

	client := &TeleportEventsWatcher{
		client: teleportEventWatcher,
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
	e := []events.AuditEvent{
		&events.UserCreate{
			Metadata: events.Metadata{
				ID: "1",
			},
		},
		&events.UserDelete{
			Metadata: events.Metadata{
				ID: "",
			},
		},
	}

	client := newTeleportEventWatcher(t, e, true)
	chEvt, chErr := client.Events(context.Background())

	select {
	case err := <-chErr:
		require.Fail(t, "received unexpected error from error channel", "error: %v", err)
	case e := <-chEvt:
		require.NotNil(t, e.Event)
		require.Equal(t, "1", e.ID)
	case <-time.After(time.Second):
		require.Fail(t, "No events were sent")
	}

	select {
	case err := <-chErr:
		require.Fail(t, "received unexpected error from error channel", "error: %v", err)
	case e := <-chEvt:
		require.NotNil(t, e.Event)
		require.Equal(t, "081ca05eea09ac0cd06e2d2acd06bec424146b254aa500de37bdc2c2b0a4dd0f", e.ID)
	case <-time.After(time.Second):
		require.Fail(t, "No events were sent")
	}
}

// errMockTeleportEventWatcher is Teleport client mock that returns an error after the first SearchUnstructuredEvents
type errMockTeleportEventWatcher struct {
	// events is the mock list of events
	events []events.AuditEvent
	t      *testing.T
	called bool
}

func (c *errMockTeleportEventWatcher) SearchEvents(ctx context.Context, fromUTC, toUTC time.Time, namespace string, eventTypes []string, limit int, order types.EventOrder, startKey string) ([]events.AuditEvent, string, error) {
	e := c.events
	c.events = nil
	return e, "test", nil
}

func (c *errMockTeleportEventWatcher) StreamSessionEvents(ctx context.Context, sessionID string, startIndex int64) (chan events.AuditEvent, chan error) {
	return nil, nil
}

func (c *errMockTeleportEventWatcher) SearchUnstructuredEvents(ctx context.Context, fromUTC, toUTC time.Time, namespace string, eventTypes []string, limit int, order types.EventOrder, startKey string) ([]*auditlogpb.EventUnstructured, string, error) {
	if c.called {
		return nil, "", errors.New("error")
	}

	e := c.events
	c.events = nil

	events := make([]*auditlogpb.EventUnstructured, len(e))
	for i, event := range e {
		events[i] = eventToJSON(c.t, event)
	}
	c.called = true
	return events, "", nil
}

func (c *errMockTeleportEventWatcher) StreamUnstructuredSessionEvents(ctx context.Context, sessionID string, startIndex int64) (chan *auditlogpb.EventUnstructured, chan error) {
	return nil, nil
}

func (c *errMockTeleportEventWatcher) UpsertLock(ctx context.Context, lock types.Lock) error {
	return nil
}

func (c *errMockTeleportEventWatcher) Ping(ctx context.Context) (proto.PingResponse, error) {
	return proto.PingResponse{
		ServerVersion: Version,
	}, nil
}

// Close is mock close method
func (c *errMockTeleportEventWatcher) Close() error {
	return nil
}

func newErrTeleportEventWatcher(t *testing.T, e []events.AuditEvent, exitOnLastEvent bool) *TeleportEventsWatcher {
	teleportEventWatcher := &errMockTeleportEventWatcher{events: e, t: t}

	client := &TeleportEventsWatcher{
		client: teleportEventWatcher,
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

func TestLastEvent(t *testing.T) {
	t.Run("should not leave hanging go-routines", func(t *testing.T) {
		e := []events.AuditEvent{
			&events.UserCreate{
				Metadata: events.Metadata{
					ID: "1",
				},
			},
		}

		client := newErrTeleportEventWatcher(t, e, false)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.Cleanup(cancel)

		chEvt, chErr := client.Events(ctx)

		select {
		case err := <-chErr:
			require.Fail(t, "received unexpected error from error channel", "error: %v", err)
		case e := <-chEvt:
			require.NotNil(t, e.Event)
			require.Equal(t, "1", e.ID)
		case <-time.After(time.Second):
			require.Fail(t, "No events were sent")
		}

		allDone := make(chan struct{})

		const nIters = 5
		for i := 0; i < nIters; i++ {
			go func() {
				chEvt, _ := client.Events(ctx)
				// we're assuming that a closed channel == closed goroutine
				for range chEvt {
				}
				allDone <- struct{}{}
			}()
		}

		for i := 0; i < nIters; i++ {
			select {
			case <-allDone:
			case <-ctx.Done():
				require.Fail(t, "timeout reached, some goroutines were not closed")
			}
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
