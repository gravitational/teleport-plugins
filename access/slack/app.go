package main

import (
	"context"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"

	"github.com/gravitational/trace"
)

const (
	// minServerVersion is the minimal teleport version the plugin supports.
	minServerVersion = "6.1.0-beta.1"
	// pluginName is used to tag PluginData and as a Delegator in Audit log.
	pluginName = "slack"
	// backoffMaxDelay is a maximum time GRPC client waits before reconnection attempt.
	backoffMaxDelay = time.Second * 2
	// initTimeout is used to bound execution time of health check and teleport version check.
	initTimeout = time.Second * 5
	// handlerTimeout is used to bound the execution time of watcher event handler.
	handlerTimeout = time.Second * 5
)

// App contains global application state.
type App struct {
	conf Config

	apiClient *client.Client
	bot       Bot
	mainJob   lib.ServiceJob

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

func (a *App) run(ctx context.Context) error {
	var err error

	log := logger.Get(ctx)
	log.Infof("Starting Teleport Access Slack Plugin %s:%s", Version, Gitref)

	bk := backoff.DefaultConfig
	bk.MaxDelay = backoffMaxDelay

	a.apiClient, err = client.New(client.WithDelegator(ctx, pluginName), client.Config{
		Addrs: []string{a.conf.Teleport.AuthServer},
		Credentials: []client.Credentials{client.LoadKeyPair(
			a.conf.Teleport.ClientCrt,
			a.conf.Teleport.ClientKey,
			a.conf.Teleport.RootCAs,
		)},
		DialInBackground: true,
		DialOpts:         []grpc.DialOption{grpc.WithConnectParams(grpc.ConnectParams{Backoff: bk})},
	})
	if err != nil {
		return trace.Wrap(err)
	}

	if err = a.init(ctx); err != nil {
		return trace.Wrap(err)
	}
	watcherJob := lib.NewWatcherJob(
		a.apiClient,
		types.Watch{Kinds: []types.WatchKind{types.WatchKind{Kind: types.KindAccessRequest}}},
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

func (a *App) init(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	log := logger.Get(ctx)

	var (
		err  error
		pong proto.PingResponse
	)
	if pong, err = a.checkTeleportVersion(ctx); err != nil {
		return trace.Wrap(err)
	}

	var webProxyAddr string
	if pong.ServerFeatures.AdvancedAccessWorkflows {
		webProxyAddr = pong.ProxyPublicAddr
	}
	a.bot, err = NewBot(a.conf, pong.ClusterName, webProxyAddr)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Debug("Starting Slack API health check...")
	if err = a.bot.HealthCheck(ctx); err != nil {
		return trace.Wrap(err, "Slack API health check failed")
	}

	log.Debug("Slack API health check finished ok")
	return nil
}

func (a *App) checkTeleportVersion(ctx context.Context) (proto.PingResponse, error) {
	log := logger.Get(ctx)
	log.Debug("Checking Teleport server version")
	pong, err := a.apiClient.WithCallOptions(grpc.WaitForReady(true)).Ping(ctx)
	if err != nil {
		if trace.IsNotImplemented(err) {
			return pong, trace.Wrap(err, "server version must be at least %s", minServerVersion)
		}
		log.Error("Unable to get Teleport server version")
		return pong, trace.Wrap(err)
	}
	err = lib.AssertServerVersion(pong, minServerVersion)
	return pong, trace.Wrap(err)
}

func (a *App) onWatcherEvent(ctx context.Context, event types.Event) error {
	ctx, cancel := context.WithTimeout(ctx, handlerTimeout)
	defer cancel()

	if kind := event.Resource.GetKind(); kind != types.KindAccessRequest {
		return trace.Errorf("unexpected kind %q", kind)
	}
	op := event.Type
	reqID := event.Resource.GetName()
	ctx, _ = logger.WithField(ctx, "request_id", reqID)

	switch op {
	case types.OpPut:
		ctx, log := logger.WithField(ctx, "request_op", "put")
		req, ok := event.Resource.(types.AccessRequest)
		if !ok {
			return trace.Errorf("unexpected resource type %T", event.Resource)
		}

		var err error
		switch {
		case req.GetState().IsPending():
			err = a.onPendingRequest(ctx, req)
		case req.GetState().IsApproved():
			err = a.onResolvedRequest(ctx, req)
		case req.GetState().IsDenied():
			err = a.onResolvedRequest(ctx, req)
		default:
			log.WithField("event", event).Warn("Unknown request state")
			return nil
		}

		if err != nil {
			log.WithError(err).Errorf("Failed to process request")
			return err
		}

		return nil

	case types.OpDelete:
		ctx, log := logger.WithField(ctx, "request_op", "delete")

		if err := a.onDeletedRequest(ctx, reqID); err != nil {
			log.WithError(err).Errorf("Failed to process deleted request")
			return err
		}
		return nil
	default:
		return trace.BadParameter("unexpected event operation %s", op)
	}
}

func (a *App) onPendingRequest(ctx context.Context, req types.AccessRequest) error {
	log := logger.Get(ctx)

	channels := a.getMessageRecipients(ctx, req.GetSuggestedReviewers())
	if len(channels) == 0 {
		log.Warning("No channel to post")
		return nil
	}

	reqData := RequestData{User: req.GetUser(), Roles: req.GetRoles(), RequestReason: req.GetRequestReason()}
	slackData, err := a.bot.Broadcast(ctx, channels, req.GetName(), reqData)
	if len(slackData) == 0 && err != nil {
		return trace.Wrap(err)
	}

	for _, data := range slackData {
		log.WithFields(logger.Fields{"slack_channel": data.ChannelID, "slack_timestamp": data.Timestamp}).
			Info("Successfully posted to Slack")
	}

	if err != nil {
		log.WithError(err).Error("Failed to post one or more messages to Mattermost")
	}

	if err := a.setPluginData(ctx, req.GetName(), PluginData{reqData, slackData}); err != nil {
		if trace.IsNotFound(err) {
			return trace.Wrap(err, "failed to save plugin data, perhaps due to lack of permissions")
		}
		return trace.Wrap(err, "failed to save plugin data")
	}

	return nil
}

func (a *App) onResolvedRequest(ctx context.Context, req types.AccessRequest) error {
	switch req.GetState() {
	case types.RequestState_APPROVED:
		return a.updateMessages(ctx, req.GetName(), "APPROVED")
	case types.RequestState_DENIED:
		return a.updateMessages(ctx, req.GetName(), "DENIED")
	default:
		return nil
	}
}

func (a *App) onDeletedRequest(ctx context.Context, reqID string) error {
	return a.updateMessages(ctx, reqID, "EXPIRED")
}

func (a *App) tryLookupDirectChannelByEmail(ctx context.Context, userEmail string) string {
	log := logger.Get(ctx)
	channel, err := a.bot.LookupDirectChannelByEmail(ctx, userEmail)
	if err != nil {
		if err.Error() == "users_not_found" {
			log.Warningf("Failed to find a user with email %q in Slack", userEmail)
		} else {
			log.WithError(err).Errorf("Failed to load user profile by email %q", userEmail)
		}
		return ""
	}
	return channel
}

func (a *App) getMessageRecipients(ctx context.Context, suggestedReviewers []string) []string {
	log := logger.Get(ctx)

	channelSet := make(map[string]struct{})
	for _, recipient := range suggestedReviewers {
		// We require SuggestedReviewers to contain email-like data. Anything else is not supported.
		if !lib.IsEmail(recipient) {
			log.Warningf("Failed to notify a suggested reviewer: %q does not look like a valid email", recipient)
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
		if channel == "" {
			continue
		}
		channelSet[channel] = struct{}{}
	}

	var channels []string
	for channel := range channelSet {
		channels = append(channels, channel)
	}
	return channels
}

func (a *App) updateMessages(ctx context.Context, reqID string, status string) error {
	log := logger.Get(ctx)

	pluginData, err := a.getPluginData(ctx, reqID)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).Warn("Cannot process unknown request")
			return nil
		}
		return trace.Wrap(err)
	}

	reqData, slackData := pluginData.RequestData, pluginData.SlackData
	if len(slackData) == 0 {
		log.Warn("Failed to update messages. Plugin data is either missing or expired")
		return nil
	}

	if err := a.bot.UpdateMessages(ctx, reqID, reqData, slackData, status); err != nil {
		return trace.Wrap(err)
	}

	log.Infof("Successfully marked request as %s in all messages", status)

	return nil
}

func (a *App) getPluginData(ctx context.Context, reqID string) (PluginData, error) {
	data, err := a.apiClient.GetPluginData(ctx, types.PluginDataFilter{
		Kind:     types.KindAccessRequest,
		Resource: reqID,
		Plugin:   pluginName,
	})
	if err != nil {
		return PluginData{}, trace.Wrap(err)
	}
	if len(data) == 0 {
		return PluginData{}, nil
	}
	entry := data[0].Entries()[pluginName]
	if entry == nil {
		return PluginData{}, nil
	}
	return DecodePluginData(entry.Data), nil
}

func (a *App) setPluginData(ctx context.Context, reqID string, data PluginData) error {
	return a.apiClient.UpdatePluginData(ctx, types.PluginDataUpdateParams{
		Kind:     types.KindAccessRequest,
		Resource: reqID,
		Plugin:   pluginName,
		Set:      EncodePluginData(data),
	})
}
