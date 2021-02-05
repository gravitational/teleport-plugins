package main

import (
	"context"
	"net/url"
	"regexp"
	"strings"
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
	webhookSrv   *WebhookServer
	mainJob      lib.ServiceJob

	*lib.Process
}

func NewApp(conf Config) (*App, error) {
	app := &App{conf: conf}
	app.mainJob = lib.NewServiceJob(app.run)
	return app, nil
}

var emailRegex = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

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
	return a.webhookSrv.BaseURL()
}

func (a *App) run(ctx context.Context) error {
	var err error

	log := logger.Get(ctx)
	log.Infof("Starting Teleport Access PagerDuty extension %s:%s", Version, Gitref)

	a.webhookSrv, err = NewWebhookServer(a.conf.HTTP, a.onPagerdutyAction)
	if err != nil {
		return trace.Wrap(err)
	}

	a.bot = NewBot(a.conf.Pagerduty, a.webhookSrv)

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
		"pagerduty",
		a.conf.Teleport.AuthServer,
		tlsConf,
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: bk,
		}),
	)
	if err != nil {
		return trace.Wrap(err)
	}
	if err = a.checkTeleportVersion(ctx); err != nil {
		return trace.Wrap(err)
	}

	log.Debug("Starting PagerDuty API health check...")
	if err = a.bot.HealthCheck(ctx); err != nil {
		log.WithError(err).Error("PagerDuty API health check failed")
		return trace.Wrap(err)
	}
	log.Debug("PagerDuty API health check finished ok")

	err = a.webhookSrv.EnsureCert()
	if err != nil {
		return trace.Wrap(err)
	}
	httpJob := a.webhookSrv.ServiceJob()
	a.SpawnCriticalJob(httpJob)
	httpOk, err := httpJob.WaitReady(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Debug("Setting up the webhook extensions")
	if err = a.bot.Setup(ctx); err != nil {
		log.WithError(err).Error("Failed to set up webhook extensions")
		return trace.Wrap(err)
	}
	log.Debug("PagerDuty webhook extensions setup finished ok")

	watcherJob := access.NewWatcherJob(
		a.accessClient,
		access.Filter{State: access.StatePending},
		a.onWatcherEvent,
	)
	a.SpawnCriticalJob(watcherJob)
	watcherOk, err := watcherJob.WaitReady(ctx)
	if err != nil {
		return trace.Wrap(err)
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

func (a *App) onPagerdutyAction(ctx context.Context, action WebhookAction) error {
	keyParts := strings.Split(action.IncidentKey, "/")
	if len(keyParts) != 2 || keyParts[0] != pdIncidentKeyPrefix {
		logger.Get(ctx).Warningf("Got unsupported incident key %q, ignoring", action.IncidentKey)
		return nil
	}

	reqID := keyParts[1]
	ctx, log := logger.WithField(ctx, "request_id", reqID)

	req, err := a.accessClient.GetRequest(ctx, reqID)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).Warning("Cannot process expired request")
			return nil
		}
		return trace.Wrap(err)
	}
	if req.State != access.StatePending {
		return trace.Errorf("cannot process not pending request: %+v", req)
	}

	ctx, log = logger.WithField(ctx, "pd_incident_id", action.IncidentID)

	pluginData, err := a.getPluginData(ctx, reqID)
	if err != nil {
		return trace.Wrap(err)
	}

	if pluginData.PagerdutyData.ID == "" {
		return trace.Errorf("plugin data is empty")
	}

	if pluginData.PagerdutyData.ID != action.IncidentID {
		log.WithField("plugin_data_incident_id", pluginData.PagerdutyData.ID).Debug("plugin_data.incident_id does not match incident.id")
		return trace.Errorf("incident_id from request's plugin_data does not match")
	}

	var userEmail, userName string
	if userID := action.Agent.ID; userID != "" {
		agent, err := a.bot.GetUserInfo(ctx, userID)
		if err != nil {
			log.WithError(err).Errorf("Cannot get user info by id %q", userID)
		} else {
			userEmail = agent.Email
			userName = agent.Name
		}
	}

	ctx, _ = logger.WithFields(ctx, logger.Fields{
		"pd_user_email": userEmail,
		"pd_user_name":  userName,
	})

	switch action.Name {
	case pdApproveAction:
		return a.setRequestState(ctx, req.ID, action.IncidentID, userEmail, access.StateApproved)
	case pdDenyAction:
		return a.setRequestState(ctx, req.ID, action.IncidentID, userEmail, access.StateDenied)
	default:
		return trace.BadParameter("unknown action: %q", action.Name)
	}
}

func (a *App) onPendingRequest(ctx context.Context, req access.Request) error {
	reqData := RequestData{User: req.User, Roles: req.Roles, Created: req.Created}

	pdData, err := a.bot.CreateIncident(ctx, req.ID, reqData)
	if err != nil {
		return trace.Wrap(err)
	}

	ctx, log := logger.WithField(ctx, "pd_incident_id", pdData.ID)

	log.Info("PagerDuty incident created")

	err = a.setPluginData(ctx, req.ID, PluginData{reqData, pdData})
	if err != nil {
		return trace.Wrap(err)
	}

	if a.conf.Pagerduty.AutoApprove {
		return a.tryAutoApproveRequest(ctx, req, pdData.ID)
	}

	return nil
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

	incidentID := pluginData.PagerdutyData.ID
	if incidentID == "" {
		log.Warn("Plugin data is either missing or expired")
		return nil
	}

	if err := a.bot.ResolveIncident(ctx, reqID, incidentID, "expired"); err != nil {
		return trace.Wrap(err)
	}

	log.Info("Successfully marked request as expired")

	return nil
}

func (a *App) setRequestState(ctx context.Context, reqID, incidentID, userEmail string, state access.State) error {
	log := logger.Get(ctx)
	var resolution string

	switch state {
	case access.StateApproved:
		resolution = "approved"
	case access.StateDenied:
		resolution = "denied"
	default:
		return trace.Errorf("unable to set state to %v", state)
	}

	if err := a.accessClient.SetRequestState(ctx, reqID, state, userEmail); err != nil {
		return trace.Wrap(err)
	}
	log.Infof("PagerDuty user %s the request", resolution)

	if err := a.bot.ResolveIncident(ctx, reqID, incidentID, resolution); err != nil {
		return trace.Wrap(err)
	}
	log.Infof("Incident %q has been resolved", incidentID)

	return nil
}

func (a *App) tryAutoApproveRequest(ctx context.Context, req access.Request, incidentID string) error {
	log := logger.Get(ctx)

	if !emailRegex.MatchString(req.User) {
		logger.Get(ctx).Warningf("Failed to auto-approve the request: %q does not look like a valid email", req.User)
		return nil
	}

	user, err := a.bot.GetUserByEmail(ctx, req.User)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).Debugf("Failed to auto-approve the request")
			return nil
		}
		return err
	}

	ctx, log = logger.WithFields(ctx, logger.Fields{
		"pd_user_email": user.Email,
		"pd_user_name":  user.Name,
	})

	isOnCall, err := a.bot.IsUserOnCall(ctx, user.ID)
	if err != nil {
		return trace.Wrap(err)
	}
	if isOnCall {
		log.Infof("User is now on-call, auto-approving the request")
		return a.setRequestState(ctx, req.ID, incidentID, user.Email, access.StateApproved)
	}

	log.Debug("User is not on call")
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
