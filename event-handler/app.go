package main

import (
	"context"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
)

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

	// Process
	*lib.Process
}

// NewApp creates new app instance
func NewApp(c *StartCmdConfig) (*App, error) {
	app := &App{config: c}
	app.mainJob = lib.NewServiceJob(app.run)
	return app, nil
}

// Run initializes and runs a watcher and a callback server
func (a *App) Run(ctx context.Context) error {
	a.Process = lib.NewProcess(ctx)
	a.SpawnCriticalJob(a.mainJob)
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

// run is the main process
func (a *App) run(ctx context.Context) error {
	log := logger.Get(ctx)

	log.WithField("version", Version).WithField("sha", Sha).Printf("Teleport event handler")

	a.config.Dump()

	err := a.init(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	t, err := NewTeleportClient(ctx, a.config, time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local), "", "")
	if err != nil {
		return trace.Wrap(err)
	}

	a.mainJob.SetReady(true)

	chEvt, chErr := t.Events()

Out:
	for {
		select {
		case err := <-chErr:
			return trace.Wrap(err)

		case evt := <-chEvt:
			if evt == nil {
				break Out
			}
			log.WithField("evt", evt).Debug("Event received")
		}
	}

	a.Terminate()
	return nil
}

// init initializes application state
func (a *App) init(ctx context.Context) error {
	a.config.Dump()

	s, err := NewState(a.config)
	if err != nil {
		return trace.Wrap(err)
	}

	a.state = s

	err = a.setStartTime(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// setStartTime sets start time or fails if start time has changed from the last run
func (a *App) setStartTime(ctx context.Context) error {
	log := logger.Get(ctx)

	prevStartTime, err := a.state.GetStartTime()
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

		return a.state.SetStartTime(t)
	}

	// If there is a time saved in the state and this time does not equal to the time passed from CLI and a
	// time was explicitly passed from CLI
	if prevStartTime != nil && a.config.StartTime != nil && *prevStartTime != *a.config.StartTime {
		return trace.Errorf("You can not change start time in the middle of ingestion. To restart the ingestion, rm -rf %v", a.config.StorageDir)
	}

	return nil
}
