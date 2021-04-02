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
const MinServerVersion = "5.0.0"

var resolveReasonInlineRegex = regexp.MustCompile(`(?im)^ *(resolution|reason) *: *(.+)$`)
var resolveReasonSeparatorRegex = regexp.MustCompile(`(?im)^ *(resolution|reason) *: *$`)

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

// Run initializes and runs a watcher and a callback server
func (a *App) Run(ctx context.Context) error {
	// Initialize the process.
	a.Process = lib.NewProcess(ctx)
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
	log := logger.Get(ctx)
	log.Infof("Starting Teleport Access JIRAbot %s:%s", Version, Gitref)

	// Create webhook server prividing a.OnJIRAWebhook as a callback function
	a.webhookSrv, err = NewWebhookServer(a.conf.HTTP, a.onJIRAWebhook)
	if err != nil {
		return
	}

	a.bot = NewBot(a.conf.JIRA)

	tlsConf, err := access.LoadTLSConfig(
		a.conf.Teleport.ClientCrt,
		a.conf.Teleport.ClientKey,
		a.conf.Teleport.RootCAs,
	)
	if trace.Unwrap(err) == access.ErrInvalidCertificate {
		log.WithError(err).Warning("Auth client TLS configuration error")
	} else if err != nil {
		return err
	}

	bk := backoff.DefaultConfig
	bk.MaxDelay = time.Second * 2
	a.accessClient, err = access.NewClient(
		ctx,
		"jira",
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

	log.Debug("Starting JIRA API health check...")
	if err = a.bot.HealthCheck(ctx); err != nil {
		log.WithError(err).Error("JIRA API health check failed")
		a.Terminate()
		return
	}
	log.Debug("JIRA API health check finished ok")

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
	ctx = logger.SetField(ctx, "request_id", req.ID)

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

// onJIRAWebhook processes JIRA webhook and updates the status of an issue
func (a *App) onJIRAWebhook(ctx context.Context, webhook Webhook) error {
	log := logger.Get(ctx)

	if webhook.WebhookEvent != "jira:issue_updated" || webhook.IssueEventTypeName != "issue_generic" {
		return nil
	}

	if webhook.Issue == nil {
		return trace.Errorf("got webhook without issue info")
	}

	issue, err := a.bot.GetIssue(ctx, webhook.Issue.ID)
	if err != nil {
		return trace.Wrap(err)
	}

	statusName := strings.ToLower(issue.Fields.Status.Name)
	if statusName == "pending" {
		log.Debug("Issue is pending, ignoring it")
		return nil
	} else if statusName == "expired" {
		log.Debug("Issue is expired, ignoring it")
		return nil
	} else if statusName != "approved" && statusName != "denied" {
		return trace.BadParameter("unknown JIRA status %q", statusName)
	}

	reqID, err := issue.GetRequestID()
	if err != nil {
		return trace.Wrap(err)
	}
	ctx, log = logger.WithField(ctx, "request_id", reqID)

	req, err := a.accessClient.GetRequest(ctx, reqID)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).Warning("Cannot process expired request")
			return nil
		}
		return trace.Wrap(err)
	}

	if req.State != access.StatePending {
		log.WithField("request_state", req.State).Warningf("Cannot process not pending request")
		return nil
	}

	pluginData, err := a.getPluginData(ctx, reqID)
	if err != nil {
		return trace.Wrap(err)
	}

	ctx, log = logger.WithFields(ctx, logger.Fields{
		"jira_issue_id":  issue.ID,
		"jira_issue_key": issue.Key,
	})

	if pluginData.JiraData.ID != issue.ID {
		log.WithField("plugin_data_issue_id", pluginData.JiraData.ID).Debug("plugin_data.issue_id does not match issue.id")
		return trace.Errorf("issue_id from request's plugin_data does not match")
	}

	var (
		params     access.RequestStateParams
		resolution string
	)

	issueUpdate, err := issue.GetLastUpdate(statusName)
	if err == nil {
		params.Delegator = issueUpdate.Author.EmailAddress

		accountID := issueUpdate.Author.AccountID
		err := a.bot.RangeIssueCommentsDescending(ctx, issue.ID, func(page PageOfComments) bool {
			for _, comment := range page.Comments {
				if comment.Author.AccountID != accountID {
					continue
				}
				contents := comment.Body
				if submatch := resolveReasonInlineRegex.FindStringSubmatch(contents); len(submatch) > 0 {
					params.Reason = strings.Trim(submatch[2], " \n")
					return false
				} else if locs := resolveReasonSeparatorRegex.FindStringIndex(contents); len(locs) > 0 {
					params.Reason = strings.TrimLeft(contents[locs[1]:], "\n")
					return false
				}
			}
			return true
		})
		if err != nil {
			log.WithError(err).Error("Cannot load issue comments")
		}
	} else {
		log.WithError(err).Error("Cannot determine who updated the issue status")
	}

	ctx, log = logger.WithFields(ctx, logger.Fields{
		"jira_user_email": issueUpdate.Author.EmailAddress,
		"jira_user_name":  issueUpdate.Author.DisplayName,
		"request_user":    req.User,
		"request_roles":   req.Roles,
		"reason":          params.Reason,
	})

	switch statusName {
	case "approved":
		params.State = access.StateApproved
		resolution = "approved"
	case "denied":
		params.State = access.StateDenied
		resolution = "denied"
	}

	if err := a.accessClient.SetRequestStateExt(ctx, req.ID, params); err != nil {
		return trace.Wrap(err)
	}
	log.Infof("JIRA user %s the request", resolution)

	return nil
}

func (a *App) onPendingRequest(ctx context.Context, req access.Request) error {
	reqData := RequestData{User: req.User, Roles: req.Roles, RequestReason: req.RequestReason, Created: req.Created}
	jiraData, err := a.bot.CreateIssue(ctx, req.ID, reqData)

	if err != nil {
		return trace.Wrap(err)
	}

	logger.Get(ctx).WithFields(logger.Fields{
		"jira_issue_id":  jiraData.ID,
		"jira_issue_key": jiraData.Key,
	}).Info("JIRA Issue created")

	err = a.setPluginData(ctx, req.ID, PluginData{reqData, jiraData})

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

	reqData, jiraData := pluginData.RequestData, pluginData.JiraData
	if jiraData.ID == "" {
		log.Warn("Plugin data is either missing or expired")
		return nil
	}

	if err := a.bot.ExpireIssue(ctx, reqID, reqData, jiraData); err != nil {
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
