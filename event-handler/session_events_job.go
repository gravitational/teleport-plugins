package main

import (
	"context"
	"time"

	ehlib "github.com/gravitational/teleport-plugins/event-handler/lib"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/backoff"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
)

const (
	// sessionBackoffBase is an initial (minimum) backoff value.
	sessionBackoffBase = 3 * time.Second
	// sessionBackoffMax is a backoff threshold
	sessionBackoffMax = 2 * time.Minute
	// sessionBackoffNumTries is the maximum number of backoff tries
	sessionBackoffNumTries = 5
)

// session is the utility struct used for session ingestion
type session struct {
	// ID current ID
	ID string
	// Index current event index
	Index int64
}

// SessionEventsJob incapsulates session events consumption logic
type SessionEventsJob struct {
	lib.ServiceJob
	app       *App
	sessions  chan session
	semaphore ehlib.Semaphore
}

// NewSessionEventsJob creates new EventsJob structure
func NewSessionEventsJob(app *App) *SessionEventsJob {
	j := &SessionEventsJob{
		app:       app,
		semaphore: ehlib.NewSemaphore(app.Config.Concurrency),
		sessions:  make(chan session),
	}

	j.ServiceJob = lib.NewServiceJob(j.run)

	return j
}

// run runs session consuming process
func (j *SessionEventsJob) run(ctx context.Context) error {
	log := logger.Get(ctx)

	// Create cancellable context which handles app termination
	process := lib.MustGetProcess(ctx)
	ctx, cancel := context.WithCancel(ctx)
	process.OnTerminate(func(_ context.Context) error {
		cancel()
		return nil
	})

	j.restartPausedSessions()

	j.SetReady(true)

	for {
		select {
		case s := <-j.sessions:
			j.semaphore.Acquire(ctx)

			log.WithField("id", s.ID).WithField("index", s.Index).Info("Starting session ingest")

			func(s session) {
				j.app.SpawnCritical(func(ctx context.Context) error {
					defer j.semaphore.Release(ctx)

					backoff := backoff.NewDecorr(sessionBackoffBase, sessionBackoffMax, clockwork.NewRealClock())
					backoffCount := sessionBackoffNumTries
					log := logger.Get(ctx).WithField("id", s.ID).WithField("index", s.Index)

					for {
						retry, err := j.consumeSession(ctx, s)

						// If sessions needs to retry
						if err != nil && retry {
							log.WithField("err", err).WithField("n", backoffCount).Error("Session ingestion error, retrying")

							// Sleep for required interval
							err := backoff.Do(ctx)
							if err != nil {
								return trace.Wrap(err)
							}

							// Check if there are number of tries left
							backoffCount--
							if backoffCount < 0 {
								log.WithField("err", err).Error("Session ingestion failed")
								return nil
							}
							continue
						}

						if err != nil {
							if !lib.IsCanceled(err) {
								log.WithField("err", err).Error("Session ingestion failed")
							}
							return err
						}

						// No errors, we've finished with this session
						return nil
					}
				})
			}(s)
		case <-ctx.Done():
			if lib.IsCanceled(ctx.Err()) {
				return nil
			}
			return ctx.Err()
		}
	}
}

// restartPausedSessions restarts sessions saved in state
func (j *SessionEventsJob) restartPausedSessions() error {
	sessions, err := j.app.State.GetSessions()
	if err != nil {
		return trace.Wrap(err)
	}

	if len(sessions) == 0 {
		return nil
	}

	for id, idx := range sessions {
		func(id string, idx int64) {
			j.app.SpawnCritical(func(ctx context.Context) error {
				log.WithField("id", id).WithField("index", idx).Info("Restarting session ingestion")

				s := session{ID: id, Index: idx}

				select {
				case j.sessions <- s:
					return nil
				case <-ctx.Done():
					if lib.IsCanceled(ctx.Err()) {
						return nil
					}

					return ctx.Err()
				}
			})
		}(id, idx)
	}

	return nil
}

// consumeSession ingests session
func (j *SessionEventsJob) consumeSession(ctx context.Context, s session) (bool, error) {
	log := logger.Get(ctx)

	url := j.app.Config.FluentdSessionURL + "." + s.ID + ".log"

	log.WithField("id", s.ID).WithField("index", s.Index).Info("Started session events ingest")
	chEvt, chErr := j.app.EventWatcher.StreamSessionEvents(ctx, s.ID, s.Index)

Loop:
	for {
		select {
		case err := <-chErr:
			return true, trace.Wrap(err)

		case evt := <-chEvt:
			if evt == nil {
				log.WithField("id", s.ID).Info("Finished session events ingest")
				break Loop // Break the main loop
			}

			e, err := NewTeleportEvent(evt, "")
			if err != nil {
				return false, trace.Wrap(err)
			}

			_, ok := j.app.Config.SkipSessionTypes[e.Type]
			if !ok {
				err := j.app.SendEvent(ctx, url, e)

				if err != nil && trace.IsConnectionProblem(err) {
					return true, trace.Wrap(err)
				}
				if err != nil {
					return false, trace.Wrap(err)
				}
			}

			// Set session index
			err = j.app.State.SetSessionIndex(s.ID, e.Index)
			if err != nil {
				return true, trace.Wrap(err)
			}
		case <-ctx.Done():
			if lib.IsCanceled(ctx.Err()) {
				return false, nil
			}

			return false, trace.Wrap(ctx.Err())
		}
	}

	// We have finished ingestion and do not need session state anymore
	err := j.app.State.RemoveSession(s.ID)
	if err != nil {
		return false, trace.Wrap(err)
	}

	return false, nil
}

// Register starts session event ingestion
func (j *SessionEventsJob) RegisterSession(ctx context.Context, e *TeleportEvent) error {
	err := j.app.State.SetSessionIndex(e.SessionID, 0)
	if err != nil {
		return trace.Wrap(err)
	}

	s := session{ID: e.SessionID, Index: 0}

	go func() error {
		select {
		case j.sessions <- s:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}()

	return nil
}
