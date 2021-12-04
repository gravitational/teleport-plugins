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
	"fmt"
	"os"
	"time"

	"github.com/gravitational/teleport-plugins/event-handler/wasm"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"github.com/wasmerio/wasmer-go/wasmer"
	"golang.org/x/net/context"
)

const (
	// lockMessage represents a message added to Lock when user is auto-locked
	lockMessage = "User is locked due to too many failed login attempts"
)

// TeleportSearchEventsClient is an interface for client.Client, required for testing
type TeleportSearchEventsClient interface {
	SearchEvents(ctx context.Context, fromUTC, toUTC time.Time, namespace string, eventTypes []string, limit int, order types.EventOrder, startKey string) ([]events.AuditEvent, string, error)
	StreamSessionEvents(ctx context.Context, sessionID string, startIndex int64) (chan events.AuditEvent, chan error)
	UpsertLock(ctx context.Context, lock types.Lock) error
	Close() error
}

// TeleportEventsWatcher represents wrapper around Teleport client to work with events
type TeleportEventsWatcher struct {
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
	batch []*TeleportEvent
	// config is teleport config
	config *StartCmdConfig
	// startTime is event time frame start
	startTime time.Time
	// wasmer instance
	wasmerInstance *wasmer.Instance
	// pluginHandleEventFn handleEvent function
	pluginHandleEventFn func(...interface{}) (interface{}, error)
	pluginNewFn         func(...interface{}) (interface{}, error)
	pluginSetFn         func(...interface{}) (interface{}, error)
}

// NewTeleportEventsWatcher builds Teleport client instance
func NewTeleportEventsWatcher(
	ctx context.Context,
	c *StartCmdConfig,
	startTime time.Time,
	cursor string,
	id string,
) (*TeleportEventsWatcher, error) {
	var err error
	var instance *wasmer.Instance

	var pluginHandleEventFn func(...interface{}) (interface{}, error)
	var pluginNewFn func(...interface{}) (interface{}, error)
	var pluginSetFn func(...interface{}) (interface{}, error)

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

	if c.Plugin != "" {
		instance, err = wasm.Init(c.Plugin)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		log.WithField("plugin", c.Plugin).Info("Using WASM plugin")

		pluginHandleEventFn, err = instance.Exports.GetFunction("handleEvent")
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if pluginHandleEventFn == nil {
			return nil, trace.BadParameter("handleEvent not found")
		}

		pluginNewFn, err = instance.Exports.GetFunction("__new")
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if pluginNewFn == nil {
			return nil, trace.BadParameter("__new not found")
		}

		pluginSetFn, err = instance.Exports.GetFunction("__set")
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if pluginSetFn == nil {
			return nil, trace.BadParameter("__set not found")
		}
	}

	tc := TeleportEventsWatcher{
		client:              client,
		pos:                 -1,
		cursor:              cursor,
		config:              c,
		id:                  id,
		startTime:           startTime,
		wasmerInstance:      instance,
		pluginHandleEventFn: pluginHandleEventFn,
		pluginNewFn:         pluginNewFn,
		pluginSetFn:         pluginSetFn,
	}

	return &tc, nil
}

// Close closes connection to Teleport
func (t *TeleportEventsWatcher) Close() {
	t.client.Close()
}

// flipPage flips the current page
func (t *TeleportEventsWatcher) flipPage() bool {
	if t.nextCursor == "" {
		return false
	}

	t.cursor = t.nextCursor
	t.pos = -1
	t.batch = make([]*TeleportEvent, 0)

	return true
}

// fetch fetches the page and sets the position to the event after latest known
func (t *TeleportEventsWatcher) fetch(ctx context.Context) error {
	log := logger.Get(ctx)

	b, nextCursor, err := t.getEvents(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	// Zero batch
	t.batch = make([]*TeleportEvent, len(b))

	// Save next cursor
	t.nextCursor = nextCursor

	// Mark position as unresolved (the page is empty)
	t.pos = -1

	log.WithField("cursor", t.cursor).WithField("next", nextCursor).WithField("len", len(b)).Debug("Fetched page")

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

	log.WithField("id", t.id).WithField("new_pos", t.pos).Debug("Skipping last known event")

	return nil
}

// getEvents calls Teleport client and loads events
func (t *TeleportEventsWatcher) getEvents(ctx context.Context) ([]events.AuditEvent, string, error) {
	return t.client.SearchEvents(
		ctx,
		t.startTime,
		time.Now().UTC(),
		t.config.Namespace,
		t.config.Types,
		t.config.BatchSize,
		types.EventOrderAscending,
		t.cursor,
	)
}

// pause sleeps for timeout seconds
func (t *TeleportEventsWatcher) pause(ctx context.Context) error {
	log := logger.Get(ctx)
	log.Debugf("No new events, pause for %v seconds", t.config.Timeout)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(t.config.Timeout):
		return nil
	}
}

// Next returns next event from a batch or requests next batch if it has been ended
func (t *TeleportEventsWatcher) Events(ctx context.Context) (chan *TeleportEvent, chan error) {
	ch := make(chan *TeleportEvent)
	e := make(chan error, 1)

	go func() {
		defer close(ch)
		defer close(e)

		for {
			// If there is nothing in the batch, request
			if len(t.batch) == 0 {
				err := t.fetch(ctx)
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

					err := t.pause(ctx)
					if err != nil {
						e <- trace.Wrap(err)
						break
					}

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
				err := t.fetch(ctx)
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

					err := t.pause(ctx)
					if err != nil {
						e <- trace.Wrap(err)
						break
					}

					continue
				}
			}

			event := t.batch[t.pos]
			t.pos++
			t.id = event.ID

			if t.pluginHandleEventFn != nil {
				log.Info("Calling plugin handle function")

				oneOf, err := events.ToOneOf(event.AuditEvent)
				if err != nil {
					e <- err
					return
				}

				var data []byte = make([]byte, oneOf.Size())
				n, err := oneOf.MarshalTo(data)
				if err != nil {
					e <- err
					return
				}

				if n > 0 {
					addr, err := t.pluginNewFn(n, 0)
					if err != nil {
						e <- err
						return
					}

					for i := 0; i < n; i++ {
						t.pluginSetFn(addr, i, data[i])
					}

					mem, err := t.wasmerInstance.Exports.GetMemory("memory")
					if err != nil {
						e <- err
						return
					}

					for i := 0; i < n; i++ {
						x := addr.(int32) + int32(i)
						if data[i] != mem.Data()[x] {
							log.Println("INVALID MEM")
							os.Exit(-1)
						}
					}
					t.pluginHandleEventFn(addr, n)
				}
			}

			select {
			case ch <- event:
			case <-ctx.Done():
				e <- ctx.Err()
				return
			}
		}
	}()

	return ch, e
}

// StreamSessionEvents returns session event stream, that's the simple delegate to an API function
func (t *TeleportEventsWatcher) StreamSessionEvents(ctx context.Context, id string, index int64) (chan events.AuditEvent, chan error) {
	return t.client.StreamSessionEvents(ctx, id, index)
}

// UpsertLock upserts user lock
func (t *TeleportEventsWatcher) UpsertLock(ctx context.Context, user string, login string, period time.Duration) error {
	var expires *time.Time

	if period > 0 {
		t := time.Now()
		t.Add(period)
		expires = &t
	}

	lock := &types.LockV2{
		Metadata: types.Metadata{
			Name: fmt.Sprintf("event-handler-auto-lock-%v-%v", user, login),
		},
		Spec: types.LockSpecV2{
			Target: types.LockTarget{
				Login: login,
				User:  user,
			},
			Message: lockMessage,
			Expires: expires,
		},
	}

	return t.client.UpsertLock(ctx, lock)
}
