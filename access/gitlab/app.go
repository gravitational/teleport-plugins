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

// App contains global application state.
type App struct {
	conf Config

	db           DB
	accessClient access.Client
	bot          *Bot
	webhookSrv   *WebhookServer
	mainJob      utils.ServiceJob

	*utils.Process
}

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
	return trace.Wrap(a.mainJob.Err())
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

func (a *App) run(ctx context.Context) (err error) {
	log.Infof("Starting Teleport Access GitLab integration %s:%s", Version, Gitref)

	a.webhookSrv, err = NewWebhookServer(
		a.conf.HTTP,
		a.conf.Gitlab.WebhookSecret,
		a.onWebhookEvent,
	)
	if err != nil {
		return
	}

	a.bot, err = NewBot(a.conf.Gitlab, a.webhookSrv)
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
		"gitlab",
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
		return
	}

	var realProjectID IntID
	log.Debug("Starting GitLab API health check...")
	realProjectID, err = a.bot.HealthCheck(ctx)
	if err != nil {
		log.Error("GitLab API health check failed")
		return
	}
	log.Debug("GitLab API health check finished ok")

	log.Debug("Opening the database...")
	a.db, err = OpenDB(a.conf.DB.Path, realProjectID)
	if err != nil {
		log.Error("Failed to open the database...")
		return
	}

	err = a.webhookSrv.EnsureCert()
	if err != nil {
		return
	}
	httpJob := a.webhookSrv.ServiceJob()
	a.SpawnCriticalJob(httpJob)
	httpOk, err := httpJob.WaitReady(ctx)
	if err != nil {
		return
	}

	log.Debug("Setting up the project")
	if err = a.setup(ctx); err != nil {
		log.Error("Failed to set up project")
		return
	}
	log.Debug("GitLab project setup finished ok")

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

	err = a.db.Close()

	return trace.NewAggregate(err, httpJob.Err(), watcherJob.Err())
}

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
	a.bot.clusterName = pong.ClusterName
	err = pong.AssertServerVersion()
	return trace.Wrap(err)
}

func (a *App) setup(ctx context.Context) error {
	return a.db.UpdateSettings(func(settings Settings) (err error) {
		webhookID := settings.HookID()
		if webhookID, err = a.bot.SetupProjectHook(ctx, webhookID); err != nil {
			return
		}
		if err = settings.SetHookID(webhookID); err != nil {
			return
		}

		labels := settings.GetLabels(
			"pending",
			"approved",
			"denied",
			"expired",
		)
		if err = a.bot.SetupLabels(ctx, labels); err != nil {
			return
		}
		if err = settings.SetLabels(a.bot.labels); err != nil {
			return
		}
		return
	})
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

func (a *App) onWebhookEvent(ctx context.Context, hook Webhook) error {
	log := log.WithFields(logFields{
		"gitlab_http_id": hook.HTTPID,
	})
	// Not an issue event
	event, ok := hook.Event.(IssueEvent)
	if !ok {
		return nil
	}
	// Non-update action
	if eventAction := event.ObjectAttributes.Action; eventAction != "update" {
		return nil
	}
	// No labels changed
	if event.Changes.Labels == nil {
		return nil
	}

	var action ActionID

	for _, label := range event.Changes.Labels.Diff() {
		action = LabelName(label.Title).ToAction()
		if action != NoAction {
			break
		}
	}
	if action == NoAction {
		log.Debug("No approved/denied labels set, ignoring")
		return nil
	}

	issueID := event.ObjectAttributes.ID
	var reqID string
	err := a.db.ViewIssues(func(issues Issues) error {
		reqID = issues.GetRequestID(issueID)
		return nil
	})
	if trace.Unwrap(err) == ErrNoBucket || reqID == "" {
		log.WithError(err).Warning("Failed to find an issue in database")
		if reqID = event.ObjectAttributes.ParseDescriptionRequestID(); reqID == "" {
			// Ignore the issue, probably it wasn't created by us at all.
			return nil
		}
		log.WithField("request_id", reqID).Warning("Request ID was parsed from issue description")
	} else if err != nil {
		return trace.Wrap(err)
	}

	req, err := a.accessClient.GetRequest(ctx, reqID)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).WithField("request_id", reqID).Warning("Cannot process expired request")
			return nil
		}
		return trace.Wrap(err)
	}
	if req.State != access.StatePending {
		return trace.Errorf("cannot process not pending request: %+v", req)
	}

	pluginData, err := a.getPluginData(ctx, reqID)
	if err != nil {
		return trace.Wrap(err)
	}

	if pluginData.GitlabData.ID == 0 {
		return trace.Errorf("plugin data is empty")
	}

	if pluginData.GitlabData.ID != issueID {
		log.WithFields(logFields{
			"gitlab_issue_id":      issueID,
			"plugin_data_issue_id": pluginData.GitlabData.ID,
		}).Debug("plugin_data.issue_id does not match event.issue_id")
		return trace.Errorf("issue_id from request's plugin_data does not match")
	}

	log = log.WithFields(logFields{
		"gitlab_project_id":    event.ObjectAttributes.ProjectID,
		"gitlab_issue_iid":     event.ObjectAttributes.IID,
		"gitlab_user_name":     event.User.Name,
		"gitlab_user_username": event.User.Username,
		"gitlab_user_email":    event.User.Email,
	})

	var (
		reqState   access.State
		resolution string
	)

	switch action {
	case ApproveAction:
		reqState = access.StateApproved
		resolution = "approved"
	case DenyAction:
		reqState = access.StateDenied
		resolution = "denied"
	default:
		return trace.BadParameter("unknown action: %v", action)
	}

	if err := a.accessClient.SetRequestState(ctx, req.ID, reqState, event.User.Email); err != nil {
		return trace.Wrap(err)
	}
	log.Infof("GitLab user %s the request", resolution)

	if err := a.bot.CloseIssue(ctx, event.ObjectAttributes.IID, ""); err != nil {
		return trace.Wrap(err)
	}
	log.Infof("Issue successfully closed")
	return nil
}

func (a *App) onPendingRequest(ctx context.Context, req access.Request) error {
	var err error
	reqData := RequestData{User: req.User, Roles: req.Roles, Created: req.Created}

	gitlabData, err := a.bot.CreateIssue(ctx, req.ID, reqData)
	if err != nil {
		return trace.Wrap(err)
	}

	log.WithFields(logFields{
		"request_id":        req.ID,
		"gitlab_project_id": gitlabData.ProjectID,
		"gitlab_issue_iid":  gitlabData.IID,
	}).Info("GitLab issue created")

	err = a.db.UpdateIssues(func(issues Issues) error {
		return issues.SetRequestID(gitlabData.ID, req.ID)
	})
	if err != nil {
		return trace.Wrap(err)
	}

	err = a.setPluginData(ctx, req.ID, PluginData{reqData, gitlabData})
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

	if err := a.bot.CloseIssue(ctx, pluginData.GitlabData.IID, "expired"); err != nil {
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
