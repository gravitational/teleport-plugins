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

	"github.com/google/go-cmp/cmp"
	"github.com/gravitational/teleport/api/types/events"
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
	testCases := []struct {
		desc   string
		events []events.AuditEvent
		want   []events.AuditEvent
	}{
		{
			desc: "test fetch events without error",
			events: []events.AuditEvent{
				&events.UserCreate{
					Metadata: events.Metadata{ID: "1"},
				},
				&events.UserDelete{
					Metadata: events.Metadata{ID: "2"},
				},
			},
			want: []events.AuditEvent{
				&events.UserCreate{
					Metadata: events.Metadata{ID: "1"},
				},
				&events.UserDelete{
					Metadata: events.Metadata{ID: "2"},
				},
			},
		},
		{
			desc:   "test no events is not an error",
			events: []events.AuditEvent{},
			want:   nil,
		},
		{
			desc: "test events without an ID are skipped",
			events: []events.AuditEvent{
				&events.UserCreate{
					Metadata: events.Metadata{},
				},
				&events.UserDelete{
					Metadata: events.Metadata{ID: "2"},
				},
			},
			want: []events.AuditEvent{
				&events.UserDelete{
					Metadata: events.Metadata{ID: "2"},
				},
			},
		},
		{
			desc:   "test SearchEvents error",
			events: []events.AuditEvent{},
			want:   nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			c := newTeleportClient(tc.events)
			var got []events.AuditEvent
			for {
				// c.Next() is considered done when there is no events and nil error.
				e, _, err := c.Next()
				if e == nil && err == nil {
					break
				}
				if err != nil {
					t.Fatalf("c.Next() error = %v, want nil error", err)
				}
				got = append(got, e)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("c.Next() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
