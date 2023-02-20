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

	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/trace"

	"github.com/gravitational/teleport-plugins/event-handler/lib"
)

const (
	// sessionEndType represents type name for session end event
	sessionEndType = "session.upload"
	// printType represents type name for print event
	printType = "print"
	// loginType represents type name for user login event
	loginType = "user.login"
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
	// SessionID is the session ID this event belongs to
	SessionID string
	// IsFailedLogin is true when this event is the failed login event
	IsFailedLogin bool
	// FailedLoginData represents failed login user data
	FailedLoginData struct {
		// Login represents user login
		Login string
		// Login represents user name
		User string
		// Login represents cluster name
		ClusterName string
	}
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
func NewTeleportEvent(e events.AuditEvent, cursor string) (*TeleportEvent, error) {
	evt := &TeleportEvent{
		Cursor: cursor,
		Type:   e.GetType(),
		Time:   e.GetTime(),
		Index:  e.GetIndex(),
	}

	err := evt.setID(e)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	evt.setSessionID(e)
	evt.setEvent(e)
	evt.setLoginData(e)

	return evt, nil
}

// setID sets or generates TeleportEvent id
func (e *TeleportEvent) setID(evt events.AuditEvent) error {
	id := evt.GetID()

	if id == "" {
		data, err := lib.FastMarshal(evt)
		if err != nil {
			return trace.Wrap(err)
		}

		hash := sha256.Sum256(data)
		id = hex.EncodeToString(hash[:])
	}

	e.ID = id

	return nil
}

// setEvent sets TeleportEvent.Event
func (e *TeleportEvent) setEvent(evt events.AuditEvent) {
	if e.Type != printType {
		e.Event = evt
		return
	}

	p := events.MustToOneOf(evt).GetSessionPrint()

	e.Event = &printEvent{
		EI:          p.GetIndex(),
		Event:       printType,
		Data:        p.Data,
		Time:        p.Time,
		ClusterName: p.ClusterName,
		CI:          p.ChunkIndex,
		Bytes:       p.Bytes,
		MS:          p.DelayMilliseconds,
		Offset:      p.Offset,
		UID:         e.ID,
	}
}

// setSessionID sets session id for session end event
func (e *TeleportEvent) setSessionID(evt events.AuditEvent) {
	if e.Type != sessionEndType {
		return
	}

	sid := events.MustToOneOf(evt).GetSessionUpload().SessionID

	e.IsSessionEnd = true
	e.SessionID = sid

}

// setLoginValues sets values related to login event
func (e *TeleportEvent) setLoginData(evt events.AuditEvent) {
	if e.Type != loginType {
		return
	}

	l := events.MustToOneOf(evt).GetUserLogin()
	if l.Success {
		return
	}

	e.IsFailedLogin = true
	e.FailedLoginData.Login = l.Login
	e.FailedLoginData.User = l.User
	e.FailedLoginData.ClusterName = l.ClusterName
}
