/*
Copyright 2020 Gravitational, Inc.

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

package events

import (
	"context"
	"time"

	"github.com/gravitational/teleport"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/teleport/lib/defaults"
	"github.com/gravitational/teleport/lib/utils"

	"github.com/gravitational/trace"

	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
)

// UploadCompleterConfig specifies configuration for the uploader
type UploadCompleterConfig struct {
	// AuditLog is used for storing logs
	AuditLog IAuditLog
	// Uploader allows the completer to list and complete uploads
	Uploader MultipartUploader
	// GracePeriod is the period after which uploads are considered
	// abandoned and will be completed
	GracePeriod time.Duration
	// Component is a component used in logging
	Component string
	// CheckPeriod is a period for checking the upload
	CheckPeriod time.Duration
	// Clock is used to override clock in tests
	Clock clockwork.Clock
	// Unstarted does not start automatic goroutine,
	// is useful when completer is embedded in another function
	Unstarted bool
}

// CheckAndSetDefaults checks and sets default values
func (cfg *UploadCompleterConfig) CheckAndSetDefaults() error {
	if cfg.Uploader == nil {
		return trace.BadParameter("missing parameter Uploader")
	}
	if cfg.GracePeriod == 0 {
		cfg.GracePeriod = defaults.UploadGracePeriod
	}
	if cfg.Component == "" {
		cfg.Component = teleport.ComponentAuth
	}
	if cfg.CheckPeriod == 0 {
		cfg.CheckPeriod = defaults.LowResPollingPeriod
	}
	if cfg.Clock == nil {
		cfg.Clock = clockwork.NewRealClock()
	}
	return nil
}

// NewUploadCompleter returns a new instance of the upload completer
// the completer has to be closed to release resources and goroutines
func NewUploadCompleter(cfg UploadCompleterConfig) (*UploadCompleter, error) {
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	u := &UploadCompleter{
		cfg: cfg,
		log: log.WithFields(log.Fields{
			trace.Component: teleport.Component(cfg.Component, "completer"),
		}),
		cancel:   cancel,
		closeCtx: ctx,
	}
	if !cfg.Unstarted {
		go u.run()
	}
	return u, nil
}

// UploadCompleter periodically scans uploads that have not been completed
// and completes them
type UploadCompleter struct {
	cfg      UploadCompleterConfig
	log      *log.Entry
	cancel   context.CancelFunc
	closeCtx context.Context
}

func (u *UploadCompleter) run() {
	ticker := u.cfg.Clock.NewTicker(u.cfg.CheckPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.Chan():
			if err := u.CheckUploads(u.closeCtx); err != nil {
				u.log.WithError(err).Warningf("Failed to check uploads.")
			}
		case <-u.closeCtx.Done():
			return
		}
	}
}

// CheckUploads fetches uploads, checks if any uploads exceed grace period
// and completes unfinished uploads
func (u *UploadCompleter) CheckUploads(ctx context.Context) error {
	uploads, err := u.cfg.Uploader.ListUploads(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	completed := 0
	for _, upload := range uploads {
		gracePoint := upload.Initiated.Add(u.cfg.GracePeriod)
		if !gracePoint.Before(u.cfg.Clock.Now()) {
			return nil
		}
		parts, err := u.cfg.Uploader.ListParts(ctx, upload)
		if err != nil {
			return trace.Wrap(err)
		}
		if len(parts) == 0 {
			continue
		}
		u.log.Debugf("Upload %v grace period is over. Trying to complete.", upload)
		if err := u.cfg.Uploader.CompleteUpload(ctx, upload, parts); err != nil {
			return trace.Wrap(err)
		}
		u.log.Debugf("Completed upload %v.", upload)
		completed++
		uploadData := u.cfg.Uploader.GetUploadMetadata(upload.SessionID)
		err = u.ensureSessionEndEvent(ctx, uploadData)
		if err != nil {
			return trace.Wrap(err)
		}
		session := &events.SessionUpload{
			Metadata: Metadata{
				Type:  SessionUploadEvent,
				Code:  SessionUploadCode,
				Index: SessionUploadIndex,
			},
			SessionMetadata: SessionMetadata{
				SessionID: string(uploadData.SessionID),
			},
			SessionURL: uploadData.URL,
		}
		err = u.cfg.AuditLog.EmitAuditEvent(ctx, session)
		if err != nil {
			return trace.Wrap(err)
		}
	}
	if completed > 0 {
		u.log.Debugf("Found %v active uploads, completed %v.", len(uploads), completed)
	}
	return nil
}

// Close closes all outstanding operations without waiting
func (u *UploadCompleter) Close() error {
	u.cancel()
	return nil
}

func (u *UploadCompleter) ensureSessionEndEvent(ctx context.Context, uploadData UploadMetadata) error {
	var serverID, clusterName, user, login, hostname, namespace, serverAddr string
	var interactive bool

	// Get session events to find fields for constructed session end
	sessionEvents, err := u.cfg.AuditLog.GetSessionEvents(defaults.Namespace, uploadData.SessionID, 0, false)
	if err != nil {
		return trace.Wrap(err)
	}
	if len(sessionEvents) == 0 {
		return nil
	}

	// Return if session.end event already exists
	for _, event := range sessionEvents {
		if event.GetType() == SessionEndEvent {
			return nil
		}
	}

	// Session start event is the first of session events
	sessionStart := sessionEvents[0]
	if sessionStart.GetType() != SessionStartEvent {
		return trace.BadParameter("invalid session, session start is not the first event")
	}

	// Set variables
	serverID = sessionStart.GetString(SessionServerHostname)
	clusterName = sessionStart.GetString(SessionClusterName)
	hostname = sessionStart.GetString(SessionServerHostname)
	namespace = sessionStart.GetString(EventNamespace)
	serverAddr = sessionStart.GetString(SessionServerAddr)
	user = sessionStart.GetString(EventUser)
	login = sessionStart.GetString(EventLogin)
	if terminalSize := sessionStart.GetString(TerminalSize); terminalSize != "" {
		interactive = true
	}

	// Get last event to get session end time
	lastEvent := sessionEvents[len(sessionEvents)-1]

	participants := getParticipants(sessionEvents)

	sessionEndEvent := &events.SessionEnd{
		Metadata: events.Metadata{
			Type:        SessionEndEvent,
			Code:        SessionEndCode,
			ClusterName: clusterName,
		},
		ServerMetadata: events.ServerMetadata{
			ServerID:        serverID,
			ServerNamespace: namespace,
			ServerHostname:  hostname,
			ServerAddr:      serverAddr,
		},
		SessionMetadata: events.SessionMetadata{
			SessionID: string(uploadData.SessionID),
		},
		UserMetadata: events.UserMetadata{
			User:  user,
			Login: login,
		},
		Participants: participants,
		Interactive:  interactive,
		StartTime:    sessionStart.GetTime(EventTime),
		EndTime:      lastEvent.GetTime(EventTime),
	}

	// Check and set event fields
	if err = checkAndSetEventFields(sessionEndEvent, u.cfg.Clock, utils.NewRealUID(), clusterName); err != nil {
		return trace.Wrap(err)
	}
	if err = u.cfg.AuditLog.EmitAuditEvent(ctx, sessionEndEvent); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func getParticipants(sessionEvents []EventFields) []string {
	var participants []string
	for _, event := range sessionEvents {
		if event.GetType() == SessionJoinEvent || event.GetType() == SessionStartEvent {
			participant := event.GetString(EventUser)
			participants = append(participants, participant)

		}
	}
	return utils.Deduplicate(participants)
}
