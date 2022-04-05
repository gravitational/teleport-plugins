package main

import (
	"context"

	"github.com/gravitational/teleport-plugin-framework/lib/wasm"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/trace"
	limiter "github.com/sethvargo/go-limiter"
	"github.com/sethvargo/go-limiter/memorystore"
)

// EventsJob incapsulates audit log event consumption logic
type EventsJob struct {
	lib.ServiceJob
	app *App
	rl  limiter.Store
}

// NewEventsJob creates new EventsJob structure
func NewEventsJob(app *App) *EventsJob {
	j := &EventsJob{app: app}
	j.ServiceJob = lib.NewServiceJob(j.run)
	return j
}

// run runs the event consumption logic
func (j *EventsJob) run(ctx context.Context) error {
	log := logger.Get(ctx)

	// Create cancellable context which handles app termination
	ctx, cancel := context.WithCancel(ctx)
	j.app.Process.OnTerminate(func(_ context.Context) error {
		cancel()
		return nil
	})

	store, err := memorystore.New(&memorystore.Config{
		Tokens:   uint64(j.app.Config.LockFailedAttemptsCount),
		Interval: j.app.Config.LockPeriod,
	})
	if err != nil {
		return trace.Wrap(err)
	}

	j.rl = store

	j.SetReady(true)

	for {
		err := j.runPolling(ctx)

		switch {
		case trace.IsConnectionProblem(err):
			log.WithError(err).Error("Failed to connect to Teleport Auth server. Reconnecting...")
		case trace.IsEOF(err):
			log.WithError(err).Error("Watcher stream closed. Reconnecting...")
		case lib.IsCanceled(err):
			log.Debug("Watcher context is cancelled")
			j.app.Terminate()
			return nil
		default:
			j.app.Terminate()
			if err == nil {
				return nil
			}
			log.WithError(err).Error("Watcher event loop failed")
			return trace.Wrap(err)
		}
	}
}

// runPolling runs actual event queue polling
func (j *EventsJob) runPolling(ctx context.Context) error {
	log := logger.Get(ctx)

	evtCh, errCh := j.app.EventWatcher.Events(ctx)

	for {
		select {
		case err := <-errCh:
			log.WithField("err", err).Error("Error ingesting Audit Log")
			return trace.Wrap(err)

		case evt := <-evtCh:
			if evt == nil {
				return nil
			}

			err := j.handleEvent(ctx, evt)
			if err != nil {
				return trace.Wrap(err)
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// handleEvent processes an event
func (j *EventsJob) handleEvent(ctx context.Context, evt *TeleportEvent) error {
	e, err := j.callPlugin(ctx, evt)
	if err != nil {
		return trace.Wrap(err)
	}

	if e == nil {
		return nil
	}

	// Send event to Teleport
	err = j.sendEvent(ctx, evt)
	if err != nil {
		return trace.Wrap(err)
	}

	// Start session ingestion if needed
	if e.IsSessionEnd {
		j.app.RegisterSession(ctx, evt)
	}

	// If the event is login event
	if e.IsFailedLogin {
		err := j.TryLockUser(ctx, e)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	// Save last event id and cursor to disk
	j.app.State.SetID(e.ID)
	j.app.State.SetCursor(e.Cursor)

	return nil
}

// callPlugin calls WASM plugin on an event
func (j *EventsJob) callPlugin(ctx context.Context, evt *TeleportEvent) (*SanitizedTeleportEvent, error) {
	if j.app.wasmHandleEvent == nil {
		return NewSanitizedTeleportEvent(evt), nil
	}

	r, err := j.app.wasmPool.Execute(ctx, func(ectx *wasm.ExecutionContext) (interface{}, error) {
		handleEvent, err := j.app.wasmHandleEvent.For(ectx)
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

// sendEvent sends an event to Teleport
func (j *EventsJob) sendEvent(ctx context.Context, evt *TeleportEvent) error {
	return j.app.SendEvent(ctx, j.app.Config.FluentdURL, evt)
}

// TryLockUser locks user if they exceeded failed attempts
func (j *EventsJob) TryLockUser(ctx context.Context, evt *SanitizedTeleportEvent) error {
	if !j.app.Config.LockEnabled || j.app.Config.DryRun {
		return nil
	}

	log := logger.Get(ctx)

	_, _, _, ok, err := j.rl.Take(ctx, evt.FailedLoginData.Login)
	if err != nil {
		return trace.Wrap(err)
	}
	if ok {
		return nil
	}

	err = j.app.EventWatcher.UpsertLock(ctx, evt.FailedLoginData.User, evt.FailedLoginData.Login, j.app.Config.LockFor)
	if err != nil {
		return trace.Wrap(err)
	}

	log.WithField("data", evt.FailedLoginData).Info("User login is locked")

	return nil
}
