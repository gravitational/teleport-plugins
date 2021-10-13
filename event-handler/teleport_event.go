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
	// sessionEndType represents type name for session end event
	sessionEndType = "session.upload"
	// printType represents type name for print event
	printType = "print"
)

// TeleportEvent represents helper struct around main audit log event
type TeleportEvent struct {
	// event is the event
	Event interface{}
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
	// IsPrint is true when this event is print
	IsPrint bool
	// SessionID is the session ID this event belongs to
	SessionID string
}

// printEvent represents an artificial print event struct which adds json-serialisable data field
type printEvent struct {
	EI          int64     `json:"ei"`
	Event       string    `json:"event"`
	Data        []byte    `json:"data"`
	Time        time.Time `json:"time"`
	ClusterName string    `json:"cluster_name"`
	CI          int64     `json:"ci"`
	Bytes       int64     `json:"bytes"`
	MS          int64     `json:"ms"`
	Offset      int64     `json:"offset"`
	UID         string    `json:"uid"`
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
		e.SetID(id)
	}

	var i interface{} = e

	t := e.GetType()
	isSessionEnd := t == sessionEndType
	if isSessionEnd {
		sid = events.MustToOneOf(e).GetSessionUpload().SessionID
	}

	if t == printType {
		p := events.MustToOneOf(e).GetSessionPrint()

		i = &printEvent{
			EI:          p.GetIndex(),
			Event:       printType,
			Data:        p.Data,
			Time:        p.Time,
			ClusterName: p.ClusterName,
			CI:          p.ChunkIndex,
			Bytes:       p.Bytes,
			MS:          p.DelayMilliseconds,
			Offset:      p.Offset,
			UID:         id,
		}
	}

	return TeleportEvent{
		Event:        i,
		ID:           id,
		Cursor:       cursor,
		Type:         t,
		Time:         e.GetTime(),
		Index:        e.GetIndex(),
		IsSessionEnd: isSessionEnd,
		SessionID:    sid,
	}, nil
}
