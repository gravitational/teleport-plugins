package main

import (
	"context"
	"net/url"
	"time"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"

	"github.com/gravitational/trace"
)

// MinServerVersion is the minimal teleport version the plugin supports.
const MinServerVersion = "4.3.0"

// App contains global application state.
type App struct {
	conf Config

	accessClient access.Client
	bot          *Bot
	callbackSrv  *CallbackServer
	mainJob      lib.ServiceJob

	*lib.Process
}

// NewApp initializes a new teleport-slack app and returns it.
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

// PublicURL checks if the app is running, and if it is —
// returns the public callback URL for Slack.
func (a *App) PublicURL() *url.URL {
	if !a.mainJob.IsReady() {
		panic("app is not running")
	}
	return a.callbackSrv.BaseURL()
}

func (a *App) run(ctx context.Context) (err error) {
	log := logger.Get(ctx)
	log.Infof("Starting Teleport Access Slackbot %s:%s", Version, Gitref)

	a.bot = NewBot(a.conf.Slack)

	// Create callback server providing a.onSlackCallback as a callback function.
	a.callbackSrv, err = NewCallbackServer(a.conf.HTTP, a.conf.Slack.Secret, a.conf.Slack.NotifyOnly, a.onSlackCallback)
	if err != nil {
		return
	}

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
	a.accessClient, err = access.NewClient(
		ctx,
		"slack",
		a.conf.Teleport.AuthServer,
		tlsConf,
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: bk,
		}),
	)
	if err != nil {
		return
	}
	if err = a.checkTeleportVersion(ctx); err != nil {
		return
	}

	err = a.callbackSrv.EnsureCert()
	if err != nil {
		return
	}
	httpJob := a.callbackSrv.ServiceJob()
	a.SpawnCriticalJob(httpJob)
	httpOk, err := httpJob.WaitReady(ctx)
	if err != nil {
		return
	}

	watcherJob := access.NewWatcherJob(
		a.accessClient,
		access.Filter{State: access.StatePending},
		a.onWatcherEvent,
	)
	a.SpawnCriticalJob(watcherJob)
	watcherOk, err := watcherJob.WaitReady(ctx)
	if err != nil {
		return
	}

	a.mainJob.SetReady(httpOk && watcherOk)

	<-httpJob.Done()
	<-watcherJob.Done()

	return trace.NewAggregate(httpJob.Err(), watcherJob.Err())
}

// checkTeleportVersion checks if the Teleport Auth server
// is compatible with this plugin version.
func (a *App) checkTeleportVersion(ctx context.Context) error {
	log := logger.Get(ctx)

	log.Debug("Checking Teleport server version")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pong, err := a.accessClient.Ping(ctx)
	if err != nil {
		if trace.IsNotImplemented(err) {
			return trace.Wrap(err, "server version must be at least %s", MinServerVersion)
		}
		log.WithError(err).Error("Unable to get Teleport server version")
		return trace.Wrap(err)
	}
	a.bot.clusterName = pong.ClusterName
	err = pong.AssertServerVersion(MinServerVersion)
	return trace.Wrap(err)
}

func (a *App) onWatcherEvent(ctx context.Context, event access.Event) error {
	req, op := event.Request, event.Type
	ctx, _ = logger.WithField(ctx, "request_id", req.ID)

	switch op {
	case access.OpPut:
		ctx, log := logger.WithField(ctx, "request_op", "put")

		if !req.State.IsPending() {
			log.WithField("event", event).Warn("non-pending request event")
			return nil
		}

		if err := a.onPendingRequest(ctx, req); err != nil {
			log := log.WithError(err)
			log.Errorf("Failed to process pending request")
			log.Debugf("%v", trace.DebugReport(err))
			return err
		}
		return nil
	case access.OpDelete:
		ctx, log := logger.WithField(ctx, "request_op", "delete")

		if err := a.onDeletedRequest(ctx, req); err != nil {
			log := log.WithError(err)
			log.Errorf("Failed to process deleted request")
			log.Debugf("%v", trace.DebugReport(err))
			return err
		}
		return nil
	default:
		return trace.BadParameter("unexpected event operation %s", op)
	}
}

// OnSlackCallback processes Slack actions and updates original Slack message with a new status
func (a *App) onSlackCallback(ctx context.Context, cb Callback) error {
	if len(cb.ActionCallback.BlockActions) != 1 {
		logger.Get(ctx).WithField("slack_block_actions", cb.ActionCallback.BlockActions).Warn("Received more than one Slack action")
		return trace.Errorf("expected exactly one block action")
	}

	action := cb.ActionCallback.BlockActions[0]
	reqID := action.Value
	actionID := action.ActionID

	var slackStatus string

	ctx, _ = logger.WithField(ctx, "request_id", reqID)
	req, err := a.accessClient.GetRequest(ctx, reqID)
	var reqData RequestData

	if err != nil {
		if trace.IsNotFound(err) {
			// Request wasn't found, need to expire it's post in Slack
			slackStatus = "EXPIRED"

			// And try to fetch its request data if it exists
			var pluginData PluginData
			pluginData, _ = a.getPluginData(ctx, reqID)
			reqData = pluginData.RequestData
		} else {
			return trace.Wrap(err)
		}
	} else {
		if req.State != access.StatePending {
			return trace.Errorf("cannot process not pending request: %+v", req)
		}

		userEmail := a.tryFetchEmail(logger.SetFields(ctx, logger.Fields{
			"slack_user":    cb.User.Name,
			"slack_channel": cb.Channel.Name,
		}), cb.User.ID)

		var (
			reqState   access.State
			resolution string
		)

		switch actionID {
		case ActionApprove:
			reqState = access.StateApproved
			slackStatus = "APPROVED"
			resolution = "approved"
		case ActionDeny:
			reqState = access.StateDenied
			slackStatus = "DENIED"
			resolution = "denied"
		default:
			return trace.BadParameter("Unknown ActionID: %s", actionID)
		}

		if err := a.accessClient.SetRequestState(ctx, req.ID, reqState, userEmail); err != nil {
			return trace.Wrap(err)
		}
		logger.Get(ctx).WithFields(
			logger.Fields{
				"slack_user_email": userEmail,
				"request_user":     req.User,
				"request_roles":    req.Roles,
			},
		).Infof("Slack user %s the request", resolution)

		// Simply fill reqData from the request itself.
		reqData = RequestData{User: req.User, Roles: req.Roles}
	}

	a.Spawn(func(ctx context.Context) error {
		ctx, log := logger.WithField(ctx, "request_id", req.ID)
		if err := a.bot.Respond(ctx, req.ID, reqData, slackStatus, cb.ResponseURL); err != nil {
			log.WithError(err).Error("Cannot update Slack message")
			return err
		}
		log.Info("Successfully updated Slack message")
		return nil
	})

	return nil
}

func (a *App) tryFetchEmail(ctx context.Context, userID string) string {
	userEmail, err := a.bot.GetUserEmail(ctx, userID)
	if err != nil {
		logger.Get(ctx).WithError(err).Warning("Failed to fetch slack user email")
	}
	return userEmail
}

func (a *App) onPendingRequest(ctx context.Context, req access.Request) error {
	reqData := RequestData{User: req.User, Roles: req.Roles}
	slackData, err := a.bot.Post(ctx, req.ID, reqData)
	if err != nil {
		return trace.Wrap(err)
	}
	logger.Get(ctx).WithFields(logger.Fields{
		"slack_channel":   slackData.ChannelID,
		"slack_timestamp": slackData.Timestamp,
	}).Info("Successfully posted to Slack")

	err = a.setPluginData(ctx, req.ID, PluginData{reqData, slackData})

	return trace.Wrap(err)
}

func (a *App) onDeletedRequest(ctx context.Context, req access.Request) error {
	log := logger.Get(ctx)
	reqID := req.ID // This is the only available field

	pluginData, err := a.getPluginData(ctx, reqID)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).Warn("Cannot expire unknown request")
			return nil
		}
		return trace.Wrap(err)
	}

	reqData, slackData := pluginData.RequestData, pluginData.SlackData
	if len(slackData.ChannelID) == 0 || len(slackData.Timestamp) == 0 {
		log.Warn("Plugin data is either missing or expired")
		return nil
	}

	if err := a.bot.Expire(ctx, reqID, reqData, slackData); err != nil {
		return trace.Wrap(err)
	}

	log.Info("Successfully marked request as expired")

	return nil
}

func (a *App) getPluginData(ctx context.Context, reqID string) (PluginData, error) {
	dataMap, err := a.accessClient.GetPluginData(ctx, reqID)
	if err != nil {
		return PluginData{}, trace.Wrap(err)
	}
	return DecodePluginData(dataMap), nil
}

func (a *App) setPluginData(ctx context.Context, reqID string, data PluginData) error {
	return a.accessClient.UpdatePluginData(ctx, reqID, EncodePluginData(data), nil)
}
