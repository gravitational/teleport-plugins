package main

import (
	"context"
	"strings"
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
const MinServerVersion = "6.1.0-beta.1"

// App contains global application state.
type App struct {
	conf Config

	accessClient access.Client
	bot          Bot
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

func (a *App) run(ctx context.Context) error {
	log := logger.Get(ctx)
	log.Infof("Starting Teleport Access Mattermost Bot %s:%s", Version, Gitref)

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
		"mattermost",
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
		webProxyAddr = pong.ProxyPublicAddr
	}
	a.bot, err = NewBot(a.conf.Mattermost, pong.ClusterName, webProxyAddr)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Debug("Starting Mattermost API health check...")
	if err = a.bot.HealthCheck(ctx); err != nil {
		return trace.Wrap(err, "api health check failed. Check your token and make sure that bot is added to your team")
	}
	log.Debug("Mattermost API health check finished ok")

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
		log.Error("Unable to get Teleport server version")
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
			log.WithError(err).Errorf("Failed to process request")
			return trace.Wrap(err)
		}

		return nil
	case access.OpDelete:
		ctx, log := logger.WithField(ctx, "request_op", "delete")

		if err := a.onDeletedRequest(ctx, req.ID); err != nil {
			log.WithError(err).Errorf("Failed to process deleted request")
			return trace.Wrap(err)
		}
		return nil
	default:
		return trace.BadParameter("unexpected event operation %s", op)
	}
}

func (a *App) onPendingRequest(ctx context.Context, req access.Request) error {
	log := logger.Get(ctx)

	channels := a.getPostRecipients(ctx, req.SuggestedReviewers)
	if len(channels) == 0 {
		log.Warning("No channel to post")
		return nil
	}

	reqData := RequestData{User: req.User, Roles: req.Roles, RequestReason: req.RequestReason}
	mmData, err := a.bot.Broadcast(ctx, channels, req.ID, reqData)
	if len(mmData) == 0 && err != nil {
		return err
	}

	for _, data := range mmData {
		logger.Get(ctx).WithField("mm_post_id", data.PostID).
			Info("Successfully posted to Mattermost")
	}

	if err != nil {
		log.WithError(err).Error("Failed to post one or more messages to Mattermost")
	}

	if err := a.setPluginData(ctx, req.ID, PluginData{reqData, mmData}); err != nil {
		if trace.IsNotFound(err) {
			return trace.Wrap(err, "failed to save plugin data, perhaps due to lack of permissions")
		}
		return trace.Wrap(err)
	}

	return nil
}

func (a *App) onResolvedRequest(ctx context.Context, req access.Request) error {
	switch req.State {
	case types.RequestState_APPROVED:
		return a.updatePosts(ctx, req.ID, "APPROVED")
	case types.RequestState_DENIED:
		return a.updatePosts(ctx, req.ID, "DENIED")
	default:
		return nil
	}
}

func (a *App) onDeletedRequest(ctx context.Context, reqID string) error {
	return a.updatePosts(ctx, reqID, "EXPIRED")
}

func (a *App) tryLookupDirectChannel(ctx context.Context, userEmail string) string {
	log := logger.Get(ctx).WithField("mm_user_email", userEmail)
	channel, err := a.bot.LookupDirectChannel(ctx, userEmail)
	if err != nil {
		if errResult, ok := trace.Unwrap(err).(*ErrorResult); ok {
			log.Warningf("Failed to lookup direct channel info: %q", errResult.Message)
		} else {
			log.WithError(err).Error("Failed to lookup direct channel info")
		}
		return ""
	}
	return channel
}

func (a *App) tryLookupChannel(ctx context.Context, team, name string) string {
	log := logger.Get(ctx).WithFields(logger.Fields{
		"mm_team":    team,
		"mm_channel": name,
	})
	channel, err := a.bot.LookupChannel(ctx, team, name)
	if err != nil {
		if errResult, ok := trace.Unwrap(err).(*ErrorResult); ok {
			log.Warningf("Failed to lookup channel info: %q", errResult.Message)
		} else {
			log.WithError(err).Error("Failed to lookup channel info")
		}
		return ""
	}
	return channel
}

func (a *App) getPostRecipients(ctx context.Context, suggestedReviewers []string) []string {
	log := logger.Get(ctx)

	channelSet := make(map[string]struct{})

	for _, recipient := range suggestedReviewers {
		// We require SuggestedReviewers to contain email-like data. Anything else is not supported.
		if !lib.IsEmail(recipient) {
			log.Warningf("Failed to notify a suggested reviewer: %q does not look like a valid email", recipient)
			continue
		}
		channel := a.tryLookupDirectChannel(ctx, recipient)
		if channel == "" {
			continue
		}
		channelSet[channel] = struct{}{}
	}

	for _, recipient := range a.conf.Mattermost.Recipients {
		var channel string
		// Recipients from config file could contain either email or team and channel names separated by '/' symbol. It's up to user what format to use.
		if lib.IsEmail(recipient) {
			channel = a.tryLookupDirectChannel(ctx, recipient)
		} else {
			parts := strings.Split(recipient, "/")
			if len(parts) == 2 {
				channel = a.tryLookupChannel(ctx, parts[0], parts[1])
			} else {
				log.Warningf("Recipient must be either a user email or a channel in the format \"team/channel\" but got %q", recipient)
			}
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

func (a *App) updatePosts(ctx context.Context, reqID string, status string) error {
	log := logger.Get(ctx)

	pluginData, err := a.getPluginData(ctx, reqID)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).Warn("Cannot process unknown request")
			return nil
		}
		return trace.Wrap(err)
	}

	reqData, mmData := pluginData.RequestData, pluginData.MattermostData
	if len(mmData) == 0 {
		log.Warn("Failed to update messages. Plugin data is either missing or expired")
		return nil
	}

	if err := a.bot.UpdatePosts(ctx, reqID, reqData, mmData, status); err != nil {
		return trace.Wrap(err)
	}

	log.Infof("Successfully marked request as %s in all messages", status)

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
