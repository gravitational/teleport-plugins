package main

import (
	"context"
	"time"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/teleport-plugins/utils"
	"google.golang.org/grpc"

	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
)

// App contains global application state.
type App struct {
	conf Config

	accessClient access.Client
	webhook      *WebhookClient
	mainJob      utils.ServiceJob

	*utils.Process
}

// NewApp initializes a new teleport-webhooks app and returns it.
func NewApp(conf Config) (*App, error) {
	app := &App{
		conf:    conf,
		webhook: NewWebhookClient(conf.Webhook),
	}
	app.mainJob = utils.NewServiceJob(app.run)
	return app, nil
}

// Run initializes and runs a watcher and a callback server
func (a *App) Run(ctx context.Context) error {
	// Initialize the process.
	a.Process = utils.NewProcess(ctx)
	a.SpawnCriticalJob(a.mainJob)
	<-a.Process.Done()
	return trace.Wrap(a.mainJob.Err())
}

// WaitReady waits for the watcher service to start up.
func (a *App) WaitReady(ctx context.Context) (bool, error) {
	return a.mainJob.WaitReady(ctx)
}

func (a *App) run(ctx context.Context) (err error) {
	log.Infof("Starting Teleport Webhooks Plugin %s:%s", Version, Gitref)

	// Load the certificates that the plugin will use to authenticate
	// itself with Teleport.
	tlsConf, err := access.LoadTLSConfig(
		a.conf.Teleport.ClientCrt,
		a.conf.Teleport.ClientKey,
		a.conf.Teleport.RootCAs,
	)
	if trace.Unwrap(err) == access.ErrInvalidCertificate {
		log.WithError(err).Warning("Auth client TLS configuration error")
	} else if err != nil {
		return
	}

	// Connect to the Teleport Auth server
	a.accessClient, err = access.NewClient(
		ctx,
		"webhooks",
		a.conf.Teleport.AuthServer,
		tlsConf,
		grpc.WithBackoffMaxDelay(time.Second*2),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	)
	if err != nil {
		return
	}

	// Check that the Teleport version compatible with the API client we're using.
	if err = a.checkTeleportVersion(ctx); err != nil {
		return
	}

	// Tell the API client that we want to watch for access request events.
	watcherJob := access.NewWatcherJob(
		a.accessClient,
		access.Filter{},
		a.onWatcherEvent,
	)

	// Start the access request watcher
	a.SpawnCriticalJob(watcherJob)
	watcherOk, err := watcherJob.WaitReady(ctx)
	if err != nil {
		return
	}

	// Set the applicataion status to Ready if the watcher successfully loaded.
	a.mainJob.SetReady(watcherOk)

	// Wait for Watcher to close it's channel.
	<-watcherJob.Done()
	return trace.Wrap(watcherJob.Err())
}

// checkTeleportVersion sends a Ping command to the Teleport Auth server
// and expects a Pong with the correct Teleport version.
func (a *App) checkTeleportVersion(ctx context.Context) error {
	log.Debug("Checking Teleport server version")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pong, err := a.accessClient.Ping(ctx)
	if err != nil {
		if trace.IsNotImplemented(err) {
			return trace.Wrap(err, "server version must be at least %s", access.MinServerVersion)
		}
		log.Error("Unable to get Teleport server version")
		return trace.Wrap(err)
	}
	err = pong.AssertServerVersion()

	// Set cluster name from Teleport Auth server
	a.webhook.clusterName = pong.ClusterName
	return trace.Wrap(err)
}

// onWatcherEvent is invoked when there's a new Access Request event.
func (a *App) onWatcherEvent(ctx context.Context, event access.Event) error {
	req, op := event.Request, event.Type
	switch op {
	case access.OpPut:
		if err := a.onRequestUpdate(ctx, req); err != nil {
			log := log.WithField("request_id", req.ID).WithError(err)
			log.Errorf("Failed to process request update event")
			log.Debugf("%v", trace.DebugReport(err))
			return err
		}
		return nil
	case access.OpDelete:
		if err := a.onDeletedRequest(ctx, req); err != nil {
			log := log.WithField("request_id", req.ID).WithError(err)
			log.Errorf("Failed to process deleted request")
			log.Debugf("%v", trace.DebugReport(err))
			return err
		}
		return nil
	default:
		return trace.BadParameter("unexpected event operation %s", op)
	}
}

// onRequestUpdate	is invoked when there's an access request update from
// the Teleport server.
// It sends the webhook with the json-marshalled request and some extra meta information.
func (a *App) onRequestUpdate(ctx context.Context, req access.Request) error {
	err := a.webhook.sendWebhook(ctx, req)
	if err != nil {
		return trace.Wrap(err)
	}

	log.WithField("request_id", req.ID).Info("Successfully posted to the Webhook")
	return nil
}

// TODO add a config option to send a webhook when a request is deleted.
func (a *App) onDeletedRequest(ctx context.Context, req access.Request) error {
	log.WithField("request_id", req.ID).Info("Ignoring deleted request")
	return nil
}
