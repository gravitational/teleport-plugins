package main

import (
	"context"
	"net/url"
	"time"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/teleport-plugins/utils"
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
	bot          *Bot
	callbackSrv  *CallbackServer
	mainJob      utils.ServiceJob

	*utils.Process
}

// NewApp initializes a new teleport-slack app and returns it.
func NewApp(conf Config) (*App, error) {
	app := &App{conf: conf}
	app.mainJob = utils.NewServiceJob(app.run)
	return app, nil
}

// Run initializes and runs a watcher and a callback server
func (a *App) Run(ctx context.Context) error {
	// Initialize the process.
	a.Process = utils.NewProcess(ctx)
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
	switch op {
	case access.OpPut:
		if !req.State.IsPending() {
			log.WithField("event", event).Warn("non-pending request event")
			return nil
		}

		if err := a.onPendingRequest(ctx, req); err != nil {
			log := log.WithField("request_id", req.ID).WithError(err)
			log.Errorf("Failed to process pending request")
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

// OnSlackCallback processes Slack actions and updates original Slack message with a new status
func (a *App) onSlackCallback(ctx context.Context, cb Callback) error {
	log := log.WithField("slack_http_id", cb.HTTPRequestID)

	if len(cb.ActionCallback.BlockActions) != 1 {
		log.WithField("slack_block_actions", cb.ActionCallback.BlockActions).Warn("Received more than one Slack action")
		return trace.Errorf("expected exactly one block action")
	}

	action := cb.ActionCallback.BlockActions[0]
	reqID := action.Value
	actionID := action.ActionID

	var slackStatus string

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

		logger := log.WithFields(logFields{
			"slack_user":    cb.User.Name,
			"slack_channel": cb.Channel.Name,
		})

		userEmail, err := a.bot.GetUserEmail(ctx, cb.User.ID)
		if err != nil {
			logger.WithError(err).Warning("Failed to fetch slack user email")
		}

		logger = logger.WithFields(logFields{
			"slack_user_email": userEmail,
			"request_id":       req.ID,
			"request_user":     req.User,
			"request_roles":    req.Roles,
		})

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
		logger.Infof("Slack user %s the request", resolution)

		// Simply fill reqData from the request itself.
		reqData = RequestData{User: req.User, Roles: req.Roles}
	}

	a.Spawn(func(ctx context.Context) error {
		if err := a.bot.Respond(ctx, req.ID, reqData, slackStatus, cb.ResponseURL); err != nil {
			log.WithError(err).WithField("request_id", req.ID).Error("Cannot update Slack message")
			return err
		}
		log.WithField("request_id", req.ID).Info("Successfully updated Slack message")
		return nil
	})

	return nil
}

func (a *App) onPendingRequest(ctx context.Context, req access.Request) error {
	reqData := RequestData{User: req.User, Roles: req.Roles}
	slackData, err := a.bot.Post(ctx, req.ID, reqData)

	if err != nil {
		return trace.Wrap(err)
	}

	log.WithFields(logFields{
		"slack_channel":   slackData.ChannelID,
		"slack_timestamp": slackData.Timestamp,
	}).Info("Successfully posted to Slack")

	err = a.setPluginData(ctx, req.ID, PluginData{reqData, slackData})

	return trace.Wrap(err)
}

func (a *App) onDeletedRequest(ctx context.Context, req access.Request) error {
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
		return trace.NotFound("plugin data was expired")
	}

	if err := a.bot.Expire(ctx, reqID, reqData, slackData); err != nil {
		return trace.Wrap(err)
	}

	log.WithField("request_id", reqID).Info("Successfully marked request as expired")

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
