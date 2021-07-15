package main

import (
	"context"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/trace"
	"golang.org/x/sync/errgroup"
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

// // WaitReady waits for http and watcher service to start up.
// func (a *App) WaitReady(ctx context.Context) (bool, error) {
// 	return a.mainJob.WaitReady(ctx)
// }

func (a *App) run(ctx context.Context) error {
	log := logger.Get(ctx)
	log.Infof("Starting Teleport event-handler")

	// a.mainJob.SetReady(true)

	// a.Terminate()

	return nil
}
