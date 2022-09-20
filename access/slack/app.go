package main

import (
	"context"
	"time"

	"google.golang.org/grpc"
	grpcbackoff "google.golang.org/grpc/backoff"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	pd "github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/gravitational/teleport-plugins/lib/stringset"
	"github.com/gravitational/teleport-plugins/lib/watcherjob"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

const (
	// minServerVersion is the minimal teleport version the plugin supports.
	minServerVersion = "6.1.0-beta.1"
	// slackPluginName is used to tag Slack PluginData and as a Delegator in Audit log.
	slackPluginName = "slack"
	// discordPluginName is used to tag Discord PluginData and as a Delegator in Audit log.
	discordPluginName = "discord"
	// grpcBackoffMaxDelay is a maximum time GRPC client waits before reconnection attempt.
	grpcBackoffMaxDelay = time.Second * 2
	// initTimeout is used to bound execution time of health check and teleport version check.
	initTimeout = time.Second * 10
	// handlerTimeout is used to bound the execution time of watcher event handler.
	handlerTimeout = time.Second * 5
)

// App contains global application state.
type App struct {
	conf Config

	apiClient *client.Client
	bot       MessagingBot
	mainJob   lib.ServiceJob
	pd        *pd.CompareAndSwap[PluginData]

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

	if err = a.init(ctx); err != nil {
		return trace.Wrap(err)
	}
	watcherJob := watcherjob.NewJob(
		a.apiClient,
		watcherjob.Config{
			Watch:            types.Watch{Kinds: []types.WatchKind{{Kind: types.KindAccessRequest}}},
			EventFuncTimeout: handlerTimeout,
		},
		a.onWatcherEvent,
	)
	a.SpawnCriticalJob(watcherJob)
	ok, err := watcherJob.WaitReady(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	a.mainJob.SetReady(ok)
	if ok {
		log.Info("Plugin is ready")
	} else {
		log.Error("Plugin is not ready")
	}

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

	bk := grpcbackoff.DefaultConfig
	bk.MaxDelay = grpcBackoffMaxDelay
	if a.apiClient, err = client.New(ctx, client.Config{
		Addrs:       a.conf.Teleport.GetAddrs(),
		Credentials: a.conf.Teleport.Credentials(),
		DialOpts: []grpc.DialOption{
			grpc.WithConnectParams(grpc.ConnectParams{Backoff: bk, MinConnectTimeout: initTimeout}),
			grpc.WithReturnConnectionError(),
		},
	}); err != nil {
		return trace.Wrap(err)
	}

	if pong, err = a.checkTeleportVersion(ctx); err != nil {
		return trace.Wrap(err)
	}

	var webProxyAddr string
	if pong.ServerFeatures.AdvancedAccessWorkflows {
		webProxyAddr = pong.ProxyPublicAddr
	}
	if a.conf.Slack.IsDiscord {
		a.bot, err = NewDiscordBot(a.conf, pong.ClusterName, webProxyAddr)
	} else {
		a.bot, err = NewSlackBot(a.conf, pong.ClusterName, webProxyAddr)
	}
	if err != nil {
		return trace.Wrap(err)
	}

	pluginName := slackPluginName
	if a.conf.Slack.IsDiscord {
		pluginName = discordPluginName
	}

	a.pd = pd.NewCAS(
		a.apiClient,
		pluginName,
		types.KindAccessRequest,
		EncodePluginData,
		DecodePluginData,
	)

	log.Debug("Starting API health check...")
	if err = a.bot.HealthCheck(ctx); err != nil {
		return trace.Wrap(err, "API health check failed")
	}

	log.Debug("API health check finished ok")
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
	if kind := event.Resource.GetKind(); kind != types.KindAccessRequest {
		return trace.Errorf("unexpected kind %s", kind)
	}
	op := event.Type
	reqID := event.Resource.GetName()
	ctx, _ = logger.WithField(ctx, "request_id", reqID)

	switch op {
	case types.OpPut:
		ctx, _ = logger.WithField(ctx, "request_op", "put")
		req, ok := event.Resource.(types.AccessRequest)
		if !ok {
			return trace.Errorf("unexpected resource type %T", event.Resource)
		}
		ctx, log := logger.WithField(ctx, "request_state", req.GetState().String())

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
			return trace.Wrap(err)
		}

		return nil
	case types.OpDelete:
		ctx, log := logger.WithField(ctx, "request_op", "delete")

		if err := a.onDeletedRequest(ctx, reqID); err != nil {
			log.WithError(err).Errorf("Failed to process deleted request")
			return trace.Wrap(err)
		}
		return nil
	default:
		return trace.BadParameter("unexpected event operation %s", op)
	}
}

func (a *App) onPendingRequest(ctx context.Context, req types.AccessRequest) error {
	log := logger.Get(ctx)

	reqID := req.GetName()
	reqData := pd.AccessRequestData{
		User:          req.GetUser(),
		Roles:         req.GetRoles(),
		RequestReason: req.GetRequestReason(),
	}

	_, err := a.pd.Create(ctx, reqID, PluginData{AccessRequestData: reqData})
	// isNew, err := a.modifyPluginData(ctx, reqID, func(existing *PluginData) (PluginData, bool) {
	// 	if existing != nil {
	// 		return PluginData{}, false
	// 	}
	// 	return PluginData{RequestData: reqData}, true
	// })
	// if err != nil {
	// 	return trace.Wrap(err)
	// }

	if !trace.IsAlreadyExists(err) {
		if err != nil {
			return trace.Wrap(err)
		}

		if channels := a.getMessageRecipients(ctx, req); len(channels) > 0 {
			if err := a.broadcastMessages(ctx, channels, reqID, reqData); err != nil {
				return trace.Wrap(err)
			}
		} else {
			log.Warning("No channel to post")
		}
	}

	if reqReviews := req.GetReviews(); len(reqReviews) > 0 {
		if a.conf.Slack.IsDiscord {
			err := a.updateMessages(ctx, reqID, pd.Unresolved, "", reqReviews)
			if err != nil {
				return trace.Wrap(err)
			}
		} else {
			if err := a.postReviewReplies(ctx, reqID, reqReviews); err != nil {
				return trace.Wrap(err)
			}
		}
	}

	return nil
}

func (a *App) onResolvedRequest(ctx context.Context, req types.AccessRequest) error {
	var replyErr error

	// Discord does not use thread replies, we do not have to send them
	if !a.conf.Slack.IsDiscord {
		if err := a.postReviewReplies(ctx, req.GetName(), req.GetReviews()); err != nil {
			replyErr = trace.Wrap(err)
		}
	}

	reason := req.GetResolveReason()
	state := req.GetState()
	var tag pd.ResolutionTag

	switch state {
	case types.RequestState_APPROVED:
		tag = pd.ResolvedApproved
	case types.RequestState_DENIED:
		tag = pd.ResolvedDenied
	default:
		logger.Get(ctx).Warningf("Unknown state %v (%s)", state, state.String())
		return replyErr
	}
	err := trace.Wrap(a.updateMessages(ctx, req.GetName(), tag, reason, req.GetReviews()))
	return trace.NewAggregate(replyErr, err)
}

func (a *App) onDeletedRequest(ctx context.Context, reqID string) error {
	return a.updateMessages(ctx, reqID, pd.ResolvedExpired, "", nil)
}

func (a *App) broadcastMessages(ctx context.Context, channels []string, reqID string, reqData pd.AccessRequestData) error {
	slackData, err := a.bot.Broadcast(ctx, channels, reqID, reqData)
	if len(slackData) == 0 && err != nil {
		return trace.Wrap(err)
	}
	for _, data := range slackData {
		logger.Get(ctx).WithFields(logger.Fields{
			"slack_channel":   data.ChannelID,
			"slack_timestamp": data.TimestampOrDiscordID,
		}).Info("Successfully posted to Slack")
	}
	if err != nil {
		logger.Get(ctx).WithError(err).Error("Failed to post one or more messages to Slack")
	}

	_, err = a.pd.Update(ctx, reqID, func(existing PluginData) (PluginData, error) {
		existing.SlackData = slackData
		return existing, nil
	})

	return trace.Wrap(err)
}

func (a *App) postReviewReplies(ctx context.Context, reqID string, reqReviews []types.AccessReview) error {
	var oldCount int
	//var slackData SlackData

	pd, err := a.pd.Update(ctx, reqID, func(existing PluginData) (PluginData, error) {
		slackData := existing.SlackData
		if len(slackData) == 0 {
			// wait for the plugin data to be updated with SlackData
			return PluginData{}, trace.CompareFailed("existing slackData is empty")
		}

		count := len(reqReviews)
		oldCount = existing.ReviewsCount
		if oldCount >= count {
			return PluginData{}, trace.AlreadyExists("reviews are sent already")
		}

		existing.ReviewsCount = count
		return existing, nil
	})
	if trace.IsAlreadyExists(err) {
		logger.Get(ctx).Debug("Failed to post reply: replies are already sent")
		return nil
	}
	if err != nil {
		return trace.Wrap(err)
	}

	slice := reqReviews[oldCount:]
	if len(slice) == 0 {
		return nil
	}

	errors := make([]error, 0, len(slice))
	for _, data := range pd.SlackData {
		ctx, _ = logger.WithFields(ctx, logger.Fields{"slack_channel": data.ChannelID, "slack_timestamp": data.TimestampOrDiscordID})
		for _, review := range slice {
			if err := a.bot.PostReviewReply(ctx, data.ChannelID, data.TimestampOrDiscordID, review); err != nil {
				errors = append(errors, err)
			}
		}
	}
	return trace.NewAggregate(errors...)
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

func (a *App) getMessageRecipients(ctx context.Context, req types.AccessRequest) []string {
	log := logger.Get(ctx)

	// We receive a set from GetRecipientsFor but we still might end up with duplicate channel names.
	// This can happen if this set contains the channel `C` and the email for channel `C`.
	channelSet := stringset.New()

	validEmaislSuggReviewers := []string{}
	for _, reviewer := range req.GetSuggestedReviewers() {
		if !lib.IsEmail(reviewer) {
			log.Warningf("Failed to notify a suggested reviewer: %q does not look like a valid email", reviewer)
			continue
		}

		validEmaislSuggReviewers = append(validEmaislSuggReviewers, reviewer)
	}

	recipients := a.conf.Recipients.GetRecipientsFor(req.GetRoles(), validEmaislSuggReviewers)
	for _, recipient := range recipients {
		// Recipients could contain either email or channel name or channel ID. It's up to user what format to use.
		channel := recipient
		if lib.IsEmail(recipient) && !a.conf.Slack.IsDiscord {
			channel = a.tryLookupDirectChannelByEmail(ctx, recipient)
		}

		if channel != "" {
			channelSet.Add(channel)
		}
	}

	return channelSet.ToSlice()
}

// updateMessages updates the messages status and adds the resolve reason.
func (a *App) updateMessages(ctx context.Context, reqID string, tag pd.ResolutionTag, reason string, reviews []types.AccessReview) error {
	log := logger.Get(ctx)

	pluginData, err := a.pd.Update(ctx, reqID, func(existing PluginData) (PluginData, error) {
		if len(existing.SlackData) == 0 {
			return PluginData{}, trace.NotFound("plugin data not found")
		}

		// If resolution field is not empty then we already resolved the incident before. In this case we just quit.
		if existing.AccessRequestData.ResolutionTag != pd.Unresolved {
			return PluginData{}, trace.CompareFailed("request is already resolved")
		}

		// Mark plugin data as resolved.
		existing.ResolutionTag = tag
		existing.ResolutionReason = reason

		return existing, nil
	})
	if trace.IsNotFound(err) {
		log.Debug("Failed to update messages: plugin data is missing")
		return nil
	}
	if err != nil {
		return trace.Wrap(err)
	}

	reqData, slackData := pluginData.AccessRequestData, pluginData.SlackData
	if err := a.bot.UpdateMessages(ctx, reqID, reqData, slackData, reviews); err != nil {
		return trace.Wrap(err)
	}

	log.Infof("Successfully marked request as %s in all messages", tag)

	return nil
}
