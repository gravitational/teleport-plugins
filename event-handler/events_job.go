package main

import (
	"context"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
)

// EventsJob incapsulates audit log event consumption logic
type EventsJob struct {
	lib.ServiceJob
	app *App
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

	evtCh, errCh := j.app.Teleport.Events(ctx)

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
	// Send event to Teleport
	err := j.sendEvent(ctx, evt)
	if err != nil {
		return trace.Wrap(err)
	}

	// Start session ingestion if needed
	if evt.IsSessionEnd {
		j.app.RegisterSession(ctx, evt)
	}

	// Save last event id and cursor to disk
	j.app.State.SetID(evt.ID)
	j.app.State.SetCursor(evt.Cursor)

	return nil
}

// sendEvent sends an event to Teleport
func (j *EventsJob) sendEvent(ctx context.Context, evt *TeleportEvent) error {
	return j.app.SendEvent(ctx, j.app.Config.FluentdURL, evt)
}
