package main

import (
	"context"
	"time"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"

	"github.com/gravitational/trace"
)

// MinServerVersion is the minimal teleport version the plugin supports.
const MinServerVersion = "6.0.0-alpha.2"

// App contains global application state.
type App struct {
	conf Config

	accessClient access.Client
	bot          *Bot
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

func (a *App) run(ctx context.Context) (err error) {
	log := logger.Get(ctx)
	log.Infof("Starting Teleport Access Slackbot %s:%s", Version, Gitref)

	tlsConf, err := access.LoadTLSConfig(
		a.conf.Teleport.ClientCrt,
		a.conf.Teleport.ClientKey,
		a.conf.Teleport.RootCAs,
	)
	if trace.Unwrap(err) == access.ErrInvalidCertificate {
		log.WithError(err).Warning("Auth client TLS configuration error")
	} else if err != nil {
		return trace.Wrap(err)
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
		return trace.Wrap(err)
	}

	var pong access.Pong
	if pong, err = a.checkTeleportVersion(ctx); err != nil {
		return trace.Wrap(err)
	}

	var webProxyAddr string
	if pong.ServerFeatures.AdvancedAccessWorkflows {
		webProxyAddr = pong.PublicProxyAddr
	}
	a.bot, err = NewBot(a.conf, pong.ClusterName, webProxyAddr)
	if err != nil {
		return trace.Wrap(err)
	}

	watcherJob := access.NewWatcherJob(
		a.accessClient,
		access.Filter{},
		a.onWatcherEvent,
	)
	a.SpawnCriticalJob(watcherJob)
	watcherOk, err := watcherJob.WaitReady(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	a.mainJob.SetReady(watcherOk)

	<-watcherJob.Done()

	return trace.Wrap(watcherJob.Err())
}

// checkTeleportVersion checks if the Teleport Auth server
// is compatible with this plugin version.
func (a *App) checkTeleportVersion(ctx context.Context) (access.Pong, error) {
	log := logger.Get(ctx)

	log.Debug("Checking Teleport server version")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pong, err := a.accessClient.Ping(ctx)
	if err != nil {
		if trace.IsNotImplemented(err) {
			return pong, trace.Wrap(err, "server version must be at least %s", MinServerVersion)
		}
		log.WithError(err).Error("Unable to get Teleport server version")
		return pong, trace.Wrap(err)
	}
	err = pong.AssertServerVersion(MinServerVersion)
	return pong, trace.Wrap(err)
}

func (a *App) onWatcherEvent(ctx context.Context, event access.Event) error {
	req, op := event.Request, event.Type
	ctx, _ = logger.WithField(ctx, "request_id", req.ID)

	switch op {
	case access.OpPut:
		ctx, log := logger.WithField(ctx, "request_op", "put")

		var err error
		switch {
		case req.State.IsPending():
			err = a.onPendingRequest(ctx, req)
		case req.State.IsApproved():
			err = a.onResolvedRequest(ctx, req)
		case req.State.IsDenied():
			err = a.onResolvedRequest(ctx, req)
		default:
			log.WithField("event", event).Warn("Unknown request state")
			return nil
		}

		if err != nil {
			log = log.WithError(err)
			log.Errorf("Failed to process updated request")
			log.Debugf("%v", trace.DebugReport(err))
			return err
		}

		return nil

	case access.OpDelete:
		ctx, log := logger.WithField(ctx, "request_op", "delete")

		if err := a.onDeletedRequest(ctx, req.ID); err != nil {
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

func (a *App) tryLookupDirectChannelByEmail(ctx context.Context, userEmail string) string {
	log := logger.Get(ctx)
	channel, err := a.bot.LookupDirectChannelByEmail(ctx, userEmail)
	if err != nil {
		if err.Error() == "users_not_found" {
			log.Warningf("User with email %q is not found in Slack", userEmail)
		} else {
			log.WithError(err).Errorf("Failed to load user profile by email %q", userEmail)
		}
		return ""
	}
	return channel
}

func (a *App) onPendingRequest(ctx context.Context, req access.Request) error {
	log := logger.Get(ctx)
	reqData := RequestData{User: req.User, Roles: req.Roles, RequestReason: req.RequestReason}

	channelSet := make(map[string]struct{})
	for _, recipient := range req.SuggestedReviewers {
		// We require SuggestedReviewers to contain email-like data. Anything else is not supported.
		if !lib.IsEmail(recipient) {
			log.Warning("Failed to notify a suggested reviewer: %q does not look like a valid email", recipient)
			continue
		}
		channel := a.tryLookupDirectChannelByEmail(ctx, recipient)
		if channel == "" {
			continue
		}
		channelSet[channel] = struct{}{}
	}
	for _, recipient := range a.conf.Slack.Recipients {
		var channel string
		// Recipients from config file could contain either email or channel name or channel ID. It's up to user what format to use.
		if lib.IsEmail(recipient) {
			channel = a.tryLookupDirectChannelByEmail(ctx, recipient)
		} else {
			channel = recipient
		}
		channelSet[channel] = struct{}{}
	}

	if len(channelSet) == 0 {
		log.Warning("no channel to post")
		return nil
	}

	var slackData SlackData
	var errors []error
	var channels []string

	for channel := range channelSet {
		channels = append(channels, channel)
	}

	slackData1, errors1 := a.bot.Broadcast(ctx, channels, req.ID, reqData)
	slackData = append(slackData, slackData1...)
	errors = append(errors, errors1...)

	if len(slackData) == 0 && len(errors) > 0 {
		return trace.Wrap(errors[0])
	}

	for _, data := range slackData {
		log.WithFields(logger.Fields{"slack_channel": data.ChannelID, "slack_timestamp": data.Timestamp}).
			Info("Successfully posted to Slack")
	}

	for _, err := range errors {
		log.WithError(err).Error("Failed to post to Slack")
	}

	if err := a.setPluginData(ctx, req.ID, PluginData{reqData, slackData}); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func (a *App) onResolvedRequest(ctx context.Context, req access.Request) error {
	switch req.State {
	case types.RequestState_APPROVED:
		return a.updateMessages(ctx, req.ID, "APPROVED")
	case types.RequestState_DENIED:
		return a.updateMessages(ctx, req.ID, "DENIED")
	default:
		return nil
	}
}

func (a *App) onDeletedRequest(ctx context.Context, reqID string) error {
	return a.updateMessages(ctx, reqID, "EXPIRED")
}

func (a *App) updateMessages(ctx context.Context, reqID string, status string) error {
	log := logger.Get(ctx)

	pluginData, err := a.getPluginData(ctx, reqID)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).Warn("Cannot expire unknown request")
			return nil
		}
		return trace.Wrap(err)
	}

	reqData, slackData := pluginData.RequestData, pluginData.SlackData
	if len(slackData) == 0 {
		log.Warn("Plugin data is either missing or expired")
		return nil
	}

	if err := a.bot.UpdateMessages(ctx, reqID, reqData, slackData, status); err != nil {
		return trace.Wrap(err)
	}

	log.Infof("Successfully marked request as %s", status)

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
