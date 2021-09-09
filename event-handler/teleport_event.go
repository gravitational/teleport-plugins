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
	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/trace"
)

const (
	// sessionEndType type name for session end event
	sessionEndType = "session.end"
)

// TeleportEvent represents helper struct around main audit log event
type TeleportEvent struct {
	// event is the event
	Event events.AuditEvent

	// cursor is the event ID (real/generated when empty)
	ID string

	// cursor is the current cursor value
	Cursor string

	// Type is an event type
	Type string

	// Time is an event timestamp
	Time time.Time

	// Index is an event index within session
	Index int64

	// IsSessionEnd is true when this event is session.end
	IsSessionEnd bool

	// SessionID is the session ID this event belongs to
	SessionID string
}

// NewTeleportEvent creates TeleportEvent using AuditEvent as a source
func NewTeleportEvent(e events.AuditEvent, cursor string) (TeleportEvent, error) {
	var sid string

	id := e.GetID()
	if id == "" {
		data, err := lib.FastMarshal(e)
		if err != nil {
			return TeleportEvent{}, trace.Wrap(err)
		}

		hash := sha256.Sum256(data)
		id = hex.EncodeToString(hash[:])
	}

	t := e.GetType()
	isSessionEnd := t == sessionEndType
	if isSessionEnd {
		sid = events.MustToOneOf(e).GetSessionEnd().SessionID
	}

	return TeleportEvent{
		Event:        e,
		ID:           id,
		Cursor:       cursor,
		Type:         t,
		Time:         e.GetTime(),
		Index:        e.GetIndex(),
		IsSessionEnd: isSessionEnd,
		SessionID:    sid,
	}, nil
}
