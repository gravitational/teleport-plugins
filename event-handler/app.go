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
	"context"
	"os"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/gravitational/teleport-plugin-framework/lib/wasm"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/backoff"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"
)

// App is the app structure
type App struct {
	// Fluentd represents the instance of Fluentd client
	Fluentd *FluentdClient
	// EventWatcher represents the instance of TeleportEventWatcher
	EventWatcher *TeleportEventsWatcher
	// State represents the instance of the persistent state
	State *State
	// cmd is start command CLI config
	Config *StartCmdConfig
	// eventsJob represents main audit log event consumer job
	eventsJob *EventsJob
	// sessionEventsJob represents session events consumer job
	sessionEventsJob *SessionEventsJob
	// WASM execution context pool
	wasmPool *wasm.ExecutionContextPool
	// wasmHandleEvent represents HandleEvent wasm bindings
	wasmHandleEvent *wasm.HandleEvent
	// badgerDB badger db
	badgerDB *badger.DB
	// Process
	*lib.Process
}

const (
	// sendBackoffBase is an initial (minimum) backoff value.
	sendBackoffBase = 1 * time.Second
	// sendBackoffMax is a backoff threshold
	sendBackoffMax = 10 * time.Second
	// sendBackoffNumTries is the maximum number of backoff tries
	sendBackoffNumTries = 5
)

// NewApp creates new app instance
func NewApp(c *StartCmdConfig) (*App, error) {
	app := &App{Config: c}

	app.eventsJob = NewEventsJob(app)
	app.sessionEventsJob = NewSessionEventsJob(app)

	return app, nil
}

// Run initializes and runs a watcher and a callback server
func (a *App) Run(ctx context.Context) error {
	a.Process = lib.NewProcess(ctx)

	err := a.init(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	a.SpawnCriticalJob(a.eventsJob)
	a.SpawnCriticalJob(a.sessionEventsJob)
	<-a.Process.Done()

	return a.Err()
}

// Err returns the error app finished with.
func (a *App) Err() error {
	return trace.NewAggregate(a.eventsJob.Err(), a.sessionEventsJob.Err())
}

// WaitReady waits for http and watcher service to start up.
func (a *App) WaitReady(ctx context.Context) (bool, error) {
	mainReady, err := a.eventsJob.WaitReady(ctx)
	if err != nil {
		return false, trace.Wrap(err)
	}

	sessionConsumerReady, err := a.sessionEventsJob.WaitReady(ctx)
	if err != nil {
		return false, trace.Wrap(err)
	}

	return mainReady && sessionConsumerReady, nil
}

// SendEvent sends an event to fluentd. Shared method used by jobs.
func (a *App) SendEvent(ctx context.Context, url string, e *SanitizedTeleportEvent) error {
	log := logger.Get(ctx)

	if !a.Config.DryRun {
		backoff := backoff.NewDecorr(sendBackoffBase, sendBackoffMax, clockwork.NewRealClock())
		backoffCount := sendBackoffNumTries

		for {
			err := a.Fluentd.Send(ctx, url, e.SanitizedEvent)
			if err == nil {
				break
			}

			log.Error("Error sending event to Teleport: ", err)

			bErr := backoff.Do(ctx)
			if bErr != nil {
				return trace.Wrap(err)
			}

			backoffCount--
			if backoffCount < 0 {
				if !lib.IsCanceled(err) {
					return trace.Wrap(err)
				}
				return nil
			}
		}
	}

	fields := logrus.Fields{"id": e.ID, "type": e.Type, "ts": e.Time, "index": e.Index}
	if e.SessionID != "" {
		fields["sid"] = e.SessionID
	}

	log.WithFields(fields).Debug("Event sent")
	log.WithField("event", e).Debug("Event dump")

	return nil
}

// init initializes application state
func (a *App) init(ctx context.Context) error {
	log := logger.Get(ctx)

	a.Config.Dump(ctx)

	s, err := NewState(a.Config)
	if err != nil {
		return trace.Wrap(err)
	}

	err = a.setStartTime(ctx, s)
	if err != nil {
		return trace.Wrap(err)
	}

	f, err := NewFluentdClient(&a.Config.FluentdConfig)
	if err != nil {
		return trace.Wrap(err)
	}

	latestCursor, err := s.GetCursor()
	if err != nil {
		return trace.Wrap(err)
	}

	latestID, err := s.GetID()
	if err != nil {
		return trace.Wrap(err)
	}

	startTime, err := s.GetStartTime()
	if err != nil {
		return trace.Wrap(err)
	}

	t, err := NewTeleportEventsWatcher(ctx, a.Config, *startTime, latestCursor, latestID)
	if err != nil {
		return trace.Wrap(err)
	}

	a.State = s
	a.Fluentd = f
	a.EventWatcher = t

	log.WithField("cursor", latestCursor).Info("Using initial cursor value")
	log.WithField("id", latestID).Info("Using initial ID value")
	log.WithField("value", startTime).Info("Using start time from state")

	err = a.initWasm(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// initWasm initializes WASM host and loads WASM plugin
func (a *App) initWasm(ctx context.Context) error {
	var err error

	log := logger.Get(ctx)

	if a.Config.WASMPlugin == "" {
		return nil
	}

	b, err := os.ReadFile(a.Config.WASMPlugin)
	if err != nil {
		return trace.Wrap(err)
	}

	a.badgerDB, err = badger.Open(badger.DefaultOptions("").WithInMemory(true))
	if err != nil {
		return trace.Wrap(err)
	}

	// Spawn badger cleanup job
	a.SpawnCritical(func(ctx context.Context) error {
		log := logger.Get(ctx)
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return trace.Wrap(ctx.Err())
			case <-ticker.C:
				log.Debug("Cleaning up badger database...")
				err := a.badgerDB.RunValueLogGC(0.7)
				if err != nil {
					return trace.Wrap(err)
				}
			}
		}
	})

	e := wasm.NewAssemblyScriptEnv(log)
	pb := wasm.NewProtobufInterop()
	s := wasm.NewStore(wasm.NewBadgerPersistentStore(a.badgerDB), wasm.DecodeAssemblyScriptString)
	a.wasmHandleEvent = wasm.NewHandleEvent(a.Config.WASMHandleEvent, pb)
	api := wasm.NewTeleportAPI(log, a.EventWatcher.client, pb)

	opts := wasm.ExecutionContextPoolOptions{
		Bytes:       b,
		Timeout:     a.Config.WASMTimeout,
		Concurrency: a.Config.WASMConcurrency,
		TraitFactories: []wasm.TraitFactory{
			e, pb, a.wasmHandleEvent, s, api,
		},
	}

	a.wasmPool, err = wasm.NewExecutionContextPool(opts)

	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// setStartTime sets start time or fails if start time has changed from the last run
func (a *App) setStartTime(ctx context.Context, s *State) error {
	log := logger.Get(ctx)

	prevStartTime, err := s.GetStartTime()
	if err != nil {
		return trace.Wrap(err)
	}

	if prevStartTime == nil {
		log.WithField("value", a.Config.StartTime).Debug("Setting start time")

		t := a.Config.StartTime
		if t == nil {
			now := time.Now().UTC().Truncate(time.Second)
			t = &now
		}

		return s.SetStartTime(t)
	}

	// If there is a time saved in the state and this time does not equal to the time passed from CLI and a
	// time was explicitly passed from CLI
	if prevStartTime != nil && a.Config.StartTime != nil && *prevStartTime != *a.Config.StartTime {
		return trace.Errorf("You can not change start time in the middle of ingestion. To restart the ingestion, rm -rf %v", a.Config.StorageDir)
	}

	return nil
}

// RegisterSession registers new session
func (a *App) RegisterSession(ctx context.Context, e *TeleportEvent) {
	a.sessionEventsJob.RegisterSession(ctx, e)
}

// CallWASMPlugin calls WASM plugin if present and returns modified event
func (a *App) CallWASMPlugin(ctx context.Context, evt *TeleportEvent) (*SanitizedTeleportEvent, error) {
	if a.wasmHandleEvent == nil {
		return NewSanitizedTeleportEvent(evt), nil
	}

	r, err := a.wasmPool.Execute(ctx, func(ectx *wasm.ExecutionContext) (interface{}, error) {
		handleEvent, err := a.wasmHandleEvent.For(ectx)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		response, err := handleEvent.HandleEvent(evt.Event)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		if response.Success == false {
			return nil, trace.Errorf(response.Error)
		}

		if response.Event == nil {
			return nil, nil
		}

		targetEvt, err := events.FromOneOf(*response.Event)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		te, err := NewTeleportEvent(targetEvt, evt.Cursor, "")
		if err != nil {
			return nil, trace.Wrap(err)
		}

		return NewSanitizedTeleportEvent(te), nil
	})

	if err != nil {
		return nil, trace.Wrap(err)
	}

	sanitized, ok := r.(*SanitizedTeleportEvent)
	if !ok {
		return nil, trace.Errorf("Can not convert %T to *SanitizedTeleportEvent", r)
	}

	return sanitized, nil
}
