package main

import (
	"context"
	"time"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/teleport-plugins/lib"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"

	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
)

// MinServerVersion is the minimal teleport version the plugin supports.
const MinServerVersion = "4.3.0"

// App contains global application state.
type App struct {
	conf Config

	accessClient access.Client
	webhook      *WebhookClient
	callbackSrv  *CallbackServer
	mainJob      lib.ServiceJob

	*lib.Process
}

// NewApp initializes a new teleport-webhooks app and returns it.
func NewApp(conf Config) (*App, error) {
	app := &App{conf: conf}
	app.mainJob = lib.NewServiceJob(app.run)
	return app, nil
}

// Run initializes and runs a watcher and a callback server
func (a *App) Run(ctx context.Context) error {
	// Initialize the process.
	a.Process = lib.NewProcess(ctx)
	a.SpawnCriticalJob(a.mainJob)
	<-a.Process.Done()
	return trace.Wrap(a.mainJob.Err())
}

// WaitReady waits for the watcher service to start up.
func (a *App) WaitReady(ctx context.Context) (bool, error) {
	return a.mainJob.WaitReady(ctx)
}

// initCallbackServer initializes the incoming webhooks (callbacks) server
// and returns it's services job, status, and, optionally, an error.
// It's invoked in `run`, only if `a.conf.Webhook.NotifyOnly` is false.
func (a *App) initCallbackServer(ctx context.Context) (lib.ServiceJob, bool, error) {
	// Make the instance of callback server first, make sure the
	// config is OK
	var err error
	a.callbackSrv, err = NewCallbackServer(a.conf.HTTP, a.onCallback)
	if err != nil {
		return nil, false, err
	}

	// Make sure the certificates are valid if the config requires them.
	err = a.callbackSrv.EnsureCert()
	if err != nil {
		return nil, false, err
	}

	// Start the server
	httpJob := a.callbackSrv.ServiceJob()
	a.SpawnCriticalJob(httpJob)
	httpOk, err := httpJob.WaitReady(ctx)
	if err != nil {
		return httpJob, httpOk, err
	}

	return httpJob, httpOk, nil
}

func (a *App) run(ctx context.Context) (err error) {
	log.Infof("Starting Teleport Webhooks Plugin %s:%s", Version, Gitref)

	// Initialize the callback server if we need to:
	// Only init the callback server if NOT running in notifyOnly mode
	var httpJob lib.ServiceJob
	httpOk := true
	if !a.conf.Webhook.NotifyOnly {
		httpJob, httpOk, err = a.initCallbackServer(ctx)
		if err != nil {
			return
		}
	}

	// Initialize the webhook sender client
	a.webhook = NewWebhookClient(a.conf)

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

	bk := backoff.DefaultConfig
	bk.MaxDelay = time.Second * 2
	// Connect to the Teleport Auth server
	a.accessClient, err = access.NewClient(
		ctx,
		"webhooks",
		a.conf.Teleport.AuthServer,
		tlsConf,
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: bk,
		}),
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
	// httpOk is true by default, so even if we haven't actually tried initializing
	// the callback server, this line will still work.
	a.mainJob.SetReady(watcherOk && httpOk)

	// Wait for Watcher to close it's channel.
	<-watcherJob.Done()

	// If running with the callback HTTP server, then also wait for it's channel to close.
	if httpJob != nil {
		<-httpJob.Done()
		return trace.NewAggregate(watcherJob.Err(), httpJob.Err())
	}
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
			return trace.Wrap(err, "server version must be at least %s", MinServerVersion)
		}
		log.Error("Unable to get Teleport server version")
		return trace.Wrap(err)
	}
	err = pong.AssertServerVersion(MinServerVersion)

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
	stateStr := stateToString(req.State)
	logger := log.WithFields(log.Fields{
		"request_id":    req.ID,
		"request_state": stateStr,
	})

	// Ignore status updates that the plugin is not configured to listen to.
	if !a.conf.Webhook.RequestStates[stateStr] {
		logger.Info("Not configured to send webhooks on this state.")
		return nil
	}

	err := a.webhook.sendWebhook(ctx, req)
	if err != nil {
		return trace.Wrap(err)
	}

	logger.Info("Successfully posted to the Webhook")
	return nil
}

func (a *App) onDeletedRequest(ctx context.Context, req access.Request) error {
	if !a.conf.Webhook.RequestStates["Deleted"] {
		log.WithField("request_id", req.ID).Info("Not configured to send webhooks on request deletions.")
		return nil
	}

	return trace.Errorf("Sending deletion requests is not implemented yet!")
}

// OnCallback processes an incoming webhook actions.
// It's called from CallbackServer's onCallback.
//
// It doesn't have to care about the HTTP request at all — just process
// the request logic, or return an error if it's invalid or an error happened.
// The CallbackServer will send the appropriate response to the HTTP request.
func (a *App) onCallback(ctx context.Context, cb Callback) error {

	// If the plugin is working in read-only mode, do not process any
	// callbacks from Slack, and return an error.
	if a.conf.Webhook.NotifyOnly {
		return trace.Errorf("Received an incoming webhook while in notify-only mode.")
	}

	// Fetch the requeast from Auth Server
	req, err := a.accessClient.GetRequest(ctx, cb.Payload.ReqID)
	if err != nil {
		return trace.Wrap(err)
	}

	// Allow only changing the requests that are pending. If someone
	// tries to change a request that's already approved or denied,
	// show them an error message.
	if req.State != access.StatePending {
		return trace.Errorf("Cannot process non-pending request: %+v", req)
	}

	// Make sure the log entries will have relevant request fields
	logger := log.WithFields(log.Fields{
		"request_id":    req.ID,
		"request_user":  req.User,
		"request_roles": req.Roles,
	})

	// Validate proposed new request state.
	newState := stringToState(cb.Payload.State)
	if newState == access.StatePending {
		logger.Debugf("Attempt to set access request state to Pending")
	}

	if err := a.accessClient.SetRequestState(ctx, req.ID, newState, cb.Payload.Delegator); err != nil {
		return trace.Wrap(err)
	}

	logger.Infof("teleport-webhook user %s %s the request", cb.Payload.Delegator, cb.Payload.State)
	return nil
}
