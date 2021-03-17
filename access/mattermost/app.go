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
	actionSrv    *ActionServer
	mainJob      lib.ServiceJob

	*lib.Process
}

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

func (a *App) PublicURL() *url.URL {
	if !a.mainJob.IsReady() {
		panic("app is not running")
	}
	return a.actionSrv.BaseURL()
}

func (a *App) run(ctx context.Context) (err error) {
	log := logger.Get(ctx)
	log.Infof("Starting Teleport Access Mattermost Bot %s:%s", Version, Gitref)

	auth := &ActionAuth{a.conf.Mattermost.Secret}

	a.actionSrv, err = NewActionServer(
		a.conf.HTTP,
		auth,
		a.onMattermostAction,
	)
	if err != nil {
		return
	}

	a.bot = NewBot(a.conf.Mattermost, a.actionSrv, auth)

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
		"mattermost",
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

	log.Debug("Starting Mattermost API health check...")
	if err = a.bot.HealthCheck(); err != nil {
		log.WithError(err).Error("Mattermost API health check failed. Check your token and make sure that bot is added to your team")
		return
	}
	log.Debug("Mattermost API health check finished ok")

	err = a.actionSrv.EnsureCert()
	if err != nil {
		return
	}
	httpJob := a.actionSrv.ServiceJob()
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
		log.Error("Unable to get Teleport server version")
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

func (a *App) onMattermostAction(ctx context.Context, data ActionData) (*ActionResponse, error) {
	action := data.Action
	reqID := data.ReqID
	ctx, _ = logger.WithField(ctx, "request_id", reqID)

	var mmStatus string

	req, err := a.accessClient.GetRequest(ctx, reqID)
	var reqData RequestData

	if err != nil {
		if trace.IsNotFound(err) {
			// Request wasn't found, need to expire it's post in Mattermost
			mmStatus = "EXPIRED"

			// And try to fetch its request data if it exists
			var pluginData PluginData
			pluginData, _ = a.getPluginData(ctx, reqID)
			reqData = pluginData.RequestData
		} else {
			return nil, trace.Wrap(err)
		}
	} else {
		ctx, log := logger.WithFields(ctx, logger.Fields{
			"mm_channel_id": data.ChannelID,
			"mm_post_id":    data.PostID,
			"mm_user_id":    data.UserID,
		})

		if req.State != access.StatePending {
			return nil, trace.Errorf("cannot process not pending request: %+v", req)
		}

		pluginData, err := a.getPluginData(ctx, reqID)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		if pluginData.MattermostData.PostID == "" {
			return nil, trace.Errorf("plugin data is empty")
		}

		if pluginData.MattermostData.PostID != data.PostID {
			log.WithField("plugin_data_post_id", pluginData.MattermostData.PostID).Debug("plugin_data.post_id does not match post.id")
			return nil, trace.Errorf("post_id from request's plugin_data does not match")
		}

		user, err := a.bot.GetUser(ctx, data.UserID)
		if err != nil {
			log.WithError(err).Warning("Failed to fetch user info")
		}
		ctx, log = logger.WithFields(ctx, logger.Fields{
			"mm_user_name":  user.Username,
			"mm_user_email": user.Email,
		})

		var (
			reqState   access.State
			resolution string
		)

		switch action {
		case "approve":
			reqState = access.StateApproved
			mmStatus = "APPROVED"
			resolution = "approved"
		case "deny":
			reqState = access.StateDenied
			mmStatus = "DENIED"
			resolution = "denied"
		default:
			return nil, trace.BadParameter("Unknown Action: %s", action)
		}

		if err := a.accessClient.SetRequestState(ctx, req.ID, reqState, user.Email); err != nil {
			return nil, trace.Wrap(err)
		}
		log.Infof("Mattermost user %s the request", resolution)

		reqData = pluginData.RequestData
	}

	return a.bot.NewActionResponse(data.PostID, reqID, reqData, mmStatus)
}

func (a *App) onPendingRequest(ctx context.Context, req access.Request) error {
	reqData := RequestData{User: req.User, Roles: req.Roles}
	mmData, err := a.bot.CreatePost(ctx, req.ID, reqData)
	if err != nil {
		return trace.Wrap(err)
	}

	logger.Get(ctx).WithField("mm_post_id", mmData.PostID).Info("Successfully posted to Mattermost")

	err = a.setPluginData(ctx, req.ID, PluginData{reqData, mmData})
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

	reqData, mmData := pluginData.RequestData, pluginData.MattermostData
	if mmData.PostID == "" {
		log.Warn("Plugin data is either missing or expired")
		return nil
	}

	if err := a.bot.ExpirePost(ctx, reqID, reqData, mmData); err != nil {
		return trace.Wrap(err)
	}

	log.Info("Successfully marked request as expired")

	return nil
}

func (a *App) getPluginData(ctx context.Context, reqID string) (data PluginData, err error) {
	dataMap, err := a.accessClient.GetPluginData(ctx, reqID)
	if err != nil {
		return PluginData{}, trace.Wrap(err)
	}
	return DecodePluginData(dataMap), nil
}

func (a *App) setPluginData(ctx context.Context, reqID string, data PluginData) error {
	return a.accessClient.UpdatePluginData(ctx, reqID, EncodePluginData(data), nil)
}
