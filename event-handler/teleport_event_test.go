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

	"github.com/gravitational/teleport/api/types/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	e := &events.SessionPrint{
		Metadata: events.Metadata{
			ID:   "test",
			Type: "mock",
		},
	}

	event, err := NewTeleportEvent(events.AuditEvent(e), "cursor")
	require.NoError(t, err)
	assert.Equal(t, "test", event.ID)
	assert.Equal(t, "mock", event.Type)
	assert.Equal(t, "cursor", event.Cursor)
}

func TestGenID(t *testing.T) {
	e := &events.SessionPrint{}

	event, err := NewTeleportEvent(events.AuditEvent(e), "cursor")
	require.NoError(t, err)
	assert.NotEmpty(t, event.ID)
}

func TestSessionEnd(t *testing.T) {
	e := &events.SessionUpload{
		Metadata: events.Metadata{
			Type: "session.upload",
		},
		SessionMetadata: events.SessionMetadata{
			SessionID: "session",
		},
	}

	event, err := NewTeleportEvent(events.AuditEvent(e), "cursor")
	require.NoError(t, err)
	assert.NotEmpty(t, event.ID)
	assert.NotEmpty(t, event.SessionID)
	assert.True(t, event.IsSessionEnd)
}

func TestFailedLogin(t *testing.T) {
	e := &events.UserLogin{
		Metadata: events.Metadata{
			Type: "user.login",
		},
		Status: events.Status{
			Success: false,
		},
	}

	event, err := NewTeleportEvent(events.AuditEvent(e), "cursor")
	require.NoError(t, err)
	assert.NotEmpty(t, event.ID)
	assert.True(t, event.IsFailedLogin)
}

func TestSuccessLogin(t *testing.T) {
	e := &events.UserLogin{
		Metadata: events.Metadata{
			Type: "user.login",
		},
		Status: events.Status{
			Success: true,
		},
	}

	event, err := NewTeleportEvent(events.AuditEvent(e), "cursor")
	require.NoError(t, err)
	assert.NotEmpty(t, event.ID)
	assert.False(t, event.IsFailedLogin)
}
