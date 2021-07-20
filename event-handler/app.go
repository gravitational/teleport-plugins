package main

import (
	"context"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

// session is the utility struct used for session ingestion
type session struct {
	// ID current ID
	ID string

	// Index current event index
	Index int
}

// App is the app structure
type App struct {
	// mainJob is the main poller loop
	mainJob lib.ServiceJob

	// fluentd is an instance of Fluentd client
	fluentd *FluentdClient

	// teleport is an instance of Teleport client
	teleport *TeleportClient

	// state is current persisted state
	state *State

	// cmd is start command CLI config
	config *StartCmdConfig

	// semaphore limiter semaphore
	semaphore chan struct{}

	// sessionIDs id queue
	sessions chan session

	// sessionConsumerJob controls session ingestion
	sessionConsumerJob lib.ServiceJob

	// Process
	*lib.Process
}

// NewApp creates new app instance
func NewApp(c *StartCmdConfig) (*App, error) {
	app := &App{config: c}
	app.mainJob = lib.NewServiceJob(app.run)
	app.sessionConsumerJob = lib.NewServiceJob(app.runSessionConsumer)
	app.semaphore = make(chan struct{}, 5) // TODO: Constant
	app.sessions = make(chan session)
	return app, nil
}

// Run initializes and runs a watcher and a callback server
func (a *App) Run(ctx context.Context) error {
	a.Process = lib.NewProcess(ctx)
	a.SpawnCriticalJob(a.mainJob)
	a.SpawnCriticalJob(a.sessionConsumerJob)
	<-a.Process.Done()
	return a.Err()
}

// Err returns the error app finished with.
func (a *App) Err() error {
	return trace.Wrap(a.mainJob.Err())
}

// WaitReady waits for http and watcher service to start up.
func (a *App) WaitReady(ctx context.Context) (bool, error) {
	return a.mainJob.WaitReady(ctx)
}

// takeSemaphore obtains semaphore
func (a *App) takeSemaphore(ctx context.Context) error {
	select {
	case a.semaphore <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// releaseSemaphore releases semaphore
func (a *App) releaseSemaphore(ctx context.Context) error {
	select {
	case <-a.semaphore:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// consumeSession ingests session
func (a *App) consumeSession(ctx context.Context, s session) error {
	log := logger.Get(ctx)

	log.WithField("id", s.ID).Info("Started session events ingest")

	url := a.config.FluentdSessionURL + "." + s.ID + ".log"

	chEvt, chErr := a.teleport.StreamSessionEvents(ctx, s.ID, int64(s.Index))

Out:
	for {
		select {
		case err := <-chErr:
			return trace.Wrap(err)

		case evt := <-chEvt:
			if evt == nil {
				log.WithField("id", s.ID).Info("Finished session events ingest")

				// Session export has finished, we do not need it's state anymore
				err := a.state.RemoveSession(s.ID)
				if err != nil {
					return trace.Wrap(err)
				}

				break Out
			}

			e, err := NewTeleportEvent(evt, "")
			if err != nil {
				return trace.Wrap(err)
			}

			_, ok := a.config.SkipSessionTypes[e.Type]
			if !ok {
				err := a.sendEvent(ctx, url, &e)
				if err != nil {
					return trace.Wrap(err)
				}
			}

			// Set session index
			err = a.state.SetSessionIndex(s.ID, e.Index)
			if err != nil {
				return trace.Wrap(err)
			}
		case <-ctx.Done():
			return nil
		}
	}

	err := a.state.RemoveSession(s.ID)
	if err != nil {
		return trace.Wrap(err)
	}

	a.releaseSemaphore(ctx)

	return nil
}

// runSessionConsumer runs session consuming process
func (a *App) runSessionConsumer(ctx context.Context) error {
	log := logger.Get(ctx)

	a.sessionConsumerJob.SetReady(true)

	for {
		select {
		case s := <-a.sessions:
			log.WithField("id", s.ID).WithField("index", s.Index).Info("Starting session ingest")

			a.takeSemaphore(ctx)
			func(s session) {
				a.SpawnCritical(func(ctx context.Context) error {
					return a.consumeSession(ctx, s)
				})
			}(s)

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// run is the main process
func (a *App) run(ctx context.Context) error {
	log := logger.Get(ctx)

	log.WithField("version", Version).WithField("sha", Sha).Printf("Teleport event handler")

	err := a.init(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	s, err := a.state.GetSessions()
	if err != nil {
		return trace.Wrap(err)
	}

	if len(s) > 0 {
		for id, idx := range s {
			func(id string, idx int) {
				a.SpawnCritical(func(ctx context.Context) error {
					log.WithField("id", id).WithField("index", idx).Info("Restarting session ingestion")

					s := session{ID: id, Index: idx}

					select {
					case a.sessions <- s:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
			}(id, int(idx))
		}
	}

	a.mainJob.SetReady(true)

	err = a.poll(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	a.Terminate()

	return nil
}

// startSessionPoll starts session event ingestion
func (a *App) startSessionPoll(ctx context.Context, e *TeleportEvent) error {
	err := a.state.SetSessionIndex(e.SessionID, 0)
	if err != nil {
		return trace.Wrap(err)
	}

	s := session{ID: e.SessionID, Index: 0}

	select {
	case a.sessions <- s:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// poll polls main audit log
func (a *App) poll(ctx context.Context) error {
	chEvt, chErr := a.teleport.Events()

	for {
		select {
		case err := <-chErr:
			return trace.Wrap(err)

		case evt := <-chEvt:
			if evt == nil {
				return nil
			}

			err := a.sendEvent(ctx, a.config.FluentdURL, evt)
			if err != nil {
				return trace.Wrap(err)
			}

			a.state.SetID(evt.ID)
			a.state.SetCursor(evt.Cursor)

			if evt.IsSessionEnd {
				func(evt *TeleportEvent) {
					a.SpawnCritical(func(ctx context.Context) error {
						return a.startSessionPoll(ctx, evt)
					})
				}(evt)
			}
		}
	}
}

// sendEvent sends an event to fluentd
func (a *App) sendEvent(ctx context.Context, url string, e *TeleportEvent) error {
	log := logger.Get(ctx)

	if !a.config.DryRun {
		err := a.fluentd.Send(url, e)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	log.WithFields(logrus.Fields{"id": e.ID, "type": e.Type, "ts": e.Time, "index": e.Index}).Info("Event sent")
	log.WithField("event", e).Debug("Event dump")

	return nil
}

// init initializes application state
func (a *App) init(ctx context.Context) error {
	log := logger.Get(ctx)

	a.config.Dump()

	s, err := NewState(a.config)
	if err != nil {
		return trace.Wrap(err)
	}

	err = a.setStartTime(ctx, s)
	if err != nil {
		return trace.Wrap(err)
	}

	f, err := NewFluentdClient(&a.config.FluentdConfig)
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

	t, err := NewTeleportClient(ctx, a.config, *startTime, latestCursor, latestID)
	if err != nil {
		return trace.Wrap(err)
	}

	a.state = s
	a.fluentd = f
	a.teleport = t

	log.WithField("cursor", latestCursor).Info("Using initial cursor value")
	log.WithField("id", latestID).Info("Using initial ID value")
	log.WithField("value", startTime).Info("Using start time from state")

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
		log.WithField("value", a.config.StartTime).Debug("Setting start time")

		t := a.config.StartTime
		if t == nil {
			now := time.Now().UTC().Truncate(time.Second)
			t = &now
		}

		return s.SetStartTime(t)
	}

	// If there is a time saved in the state and this time does not equal to the time passed from CLI and a
	// time was explicitly passed from CLI
	if prevStartTime != nil && a.config.StartTime != nil && *prevStartTime != *a.config.StartTime {
		return trace.Errorf("You can not change start time in the middle of ingestion. To restart the ingestion, rm -rf %v", a.config.StorageDir)
	}

	return nil
}
