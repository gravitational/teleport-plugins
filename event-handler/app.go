package main

import (
	"context"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

const (
	// sessionEndType type name for session end event
	sessionEndType = "session.end"
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

// runSessionIngest runs session ingestion process
func (a *App) runSessionConsumer(ctx context.Context) error {
	log := logger.Get(ctx)

	a.sessionConsumerJob.SetReady(true)

	log.Info("Session consumer started")

	for {
		select {
		case s := <-a.sessions:
			log.Infof("%v", s)
			log.Info("---------------------------")
			//a.semaphore <- struct{}{}

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

	a.mainJob.SetReady(true)

	err = a.poll(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	a.Terminate()

	return nil
}

func (a *App) startSessionPoll(ctx context.Context, evt *eventWithCursor) {
	log := logger.Get(ctx)

	e := events.MustToOneOf(evt.Event)
	id := e.GetSessionEnd().SessionID

	log.WithField("id", id).Info("Started session events ingest")

	a.sessions <- session{
		ID:    id,
		Index: 0,
	}
}

// poll polls main audit log
func (a *App) poll(ctx context.Context) error {
	log := logger.Get(ctx)

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

			if evt.Event.GetType() == sessionEndType {
				func(evt *eventWithCursor) {
					a.SpawnCritical(func(ctx context.Context) error {
						a.startSessionPoll(ctx, evt)
						return nil
					})
				}(evt)
			}

			log.WithFields(logrus.Fields{"id": evt.ID, "type": evt.Event.GetType(), "ts": evt.Event.GetTime()}).Info("Event received")
		}
	}
}

// sendEvent sends an event to fluentd
func (a *App) sendEvent(ctx context.Context, url string, e *eventWithCursor) error {
	log := logger.Get(ctx)

	if !a.config.DryRun {
		err := a.fluentd.Send(url, e)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	log.WithFields(logrus.Fields{"id": e.ID, "type": e.Event.GetType(), "ts": e.Event.GetTime(), "index": e.Event.GetIndex()}).Info("Event sent")
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

	cursor, err := s.GetCursor()
	if err != nil {
		return trace.Wrap(err)
	}

	id, err := s.GetID()
	if err != nil {
		return trace.Wrap(err)
	}

	st, err := s.GetStartTime()
	if err != nil {
		return trace.Wrap(err)
	}

	t, err := NewTeleportClient(ctx, a.config, *st, cursor, id)
	if err != nil {
		return trace.Wrap(err)
	}

	a.state = s
	a.fluentd = f
	a.teleport = t

	log.WithField("cursor", cursor).Info("Using initial cursor value")
	log.WithField("id", id).Info("Using initial ID value")
	log.WithField("value", st).Info("Using start time from state")

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
