/*
Copyright 2020-2021 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/jonboulle/clockwork"
	"google.golang.org/grpc"
	grpcbackoff "google.golang.org/grpc/backoff"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/backoff"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport-plugins/lib/plugindata"
	"github.com/gravitational/teleport-plugins/lib/watcherjob"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	apiutils "github.com/gravitational/teleport/api/utils"
	"github.com/gravitational/trace"
)

const (
	// minServerVersion is the minimal teleport version the plugin supports.
	minServerVersion = "6.1.0"
	// pluginName is used to tag PluginData and as a Delegator in Audit log.
	pluginName = "gitlab"
	// grpcBackoffMaxDelay is a maximum time GRPC client waits before reconnection attempt.
	grpcBackoffMaxDelay = time.Second * 2
	// initTimeout is used to bound execution time of health check and teleport version check.
	initTimeout = time.Second * 10
	// handlerTimeout is used to bound the execution time of watcher event handler.
	handlerTimeout = time.Second * 5
	// modifyPluginDataBackoffBase is an initial (minimum) backoff value.
	modifyPluginDataBackoffBase = time.Millisecond
	// modifyPluginDataBackoffMax is a backoff threshold
	modifyPluginDataBackoffMax = time.Second
)

// App contains global application state.
type App struct {
	conf             Config
	defaultProjectID IntID

	db         DB
	apiClient  *client.Client
	gitlab     Gitlab
	webhookSrv *WebhookServer
	mainJob    lib.ServiceJob

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

// PublicURL returns a webhook base URL.
func (a *App) PublicURL() *url.URL {
	if !a.mainJob.IsReady() {
		panic("app is not running")
	}
	return a.webhookSrv.BaseURL()
}

func (a *App) run(ctx context.Context) error {
	var err error

	log := logger.Get(ctx)
	log.Infof("Starting Teleport GitLab Plugin %s:%s", Version, Gitref)

	if err = a.init(ctx); err != nil {
		return trace.Wrap(err)
	}

	httpJob := a.webhookSrv.ServiceJob()
	a.SpawnCriticalJob(httpJob)
	httpOk, err := httpJob.WaitReady(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Debug("Setting up the project")
	if err = a.setup(ctx, a.defaultProjectID); err != nil {
		log.Error("Failed to set up project")
		return trace.Wrap(err)
	}
	log.Debug("GitLab project setup finished ok")

	watcherJob := watcherjob.NewJob(
		a.apiClient,
		watcherjob.Config{
			Watch:            types.Watch{Kinds: []types.WatchKind{types.WatchKind{Kind: types.KindAccessRequest}}},
			EventFuncTimeout: handlerTimeout,
		},
		a.onWatcherEvent,
	)
	a.SpawnCriticalJob(watcherJob)
	watcherOk, err := watcherJob.WaitReady(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	ok := httpOk && watcherOk
	a.mainJob.SetReady(ok)
	if ok {
		log.Info("Plugin is ready")
	} else {
		log.Error("Plugin is not ready")
	}

	<-httpJob.Done()
	<-watcherJob.Done()

	err = a.db.Close()

	return trace.NewAggregate(httpJob.Err(), watcherJob.Err(), err)
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
		DialOpts:    []grpc.DialOption{grpc.WithConnectParams(grpc.ConnectParams{Backoff: bk, MinConnectTimeout: initTimeout})},
	}); err != nil {
		return trace.Wrap(err)
	}

	if pong, err = a.checkTeleportVersion(ctx); err != nil {
		return trace.Wrap(err)
	}

	webhookSrv, err := NewWebhookServer(
		a.conf.HTTP,
		a.conf.Gitlab.WebhookSecret,
		a.onWebhookEvent,
	)
	if err != nil {
		return trace.Wrap(err)
	}
	err = webhookSrv.EnsureCert()
	if err != nil {
		return trace.Wrap(err)
	}

	a.webhookSrv = webhookSrv

	var webProxyAddr string
	if pong.ServerFeatures.AdvancedAccessWorkflows {
		webProxyAddr = pong.ProxyPublicAddr
	}
	a.gitlab, err = NewGitlabClient(a.conf.Gitlab, pong.ClusterName, webProxyAddr, webhookSrv)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Debug("Starting GitLab API health check...")
	a.defaultProjectID, err = a.gitlab.HealthCheck(ctx, a.conf.Gitlab.ProjectID)
	if err != nil {
		return trace.Wrap(err, "api health check failed")
	}
	log.Debug("GitLab API health check finished ok")

	log.Debug("Opening the database...")
	a.db, err = OpenDB(a.conf.DB.Path)
	if err != nil {
		return trace.Wrap(err, "failed to open the database")
	}

	return nil
}

func (a *App) checkTeleportVersion(ctx context.Context) (proto.PingResponse, error) {
	log := logger.Get(ctx)
	log.Debug("Checking Teleport server version")
	pong, err := a.apiClient.Ping(ctx)
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

func (a *App) setup(ctx context.Context, projectID IntID) error {
	return a.db.UpdateSettings(projectID, func(settings SettingsBucket) (err error) {
		webhookID := settings.HookID()
		if webhookID, err = a.gitlab.SetupProjectHook(ctx, projectID, webhookID); err != nil {
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
		if err = a.gitlab.SetupLabels(ctx, projectID, labels); err != nil {
			return
		}
		if err = settings.SetLabels(a.gitlab.labels); err != nil {
			return
		}
		return
	})
}

func (a *App) onWatcherEvent(ctx context.Context, event types.Event) error {
	if kind := event.Resource.GetKind(); kind != types.KindAccessRequest {
		return trace.Errorf("unexpected kind %q", kind)
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
		log.Debug("Processing watcher event")

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
			log.WithError(err).Error("Failed to process request")
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

func (a *App) onWebhookEvent(ctx context.Context, hook Webhook) error {
	// Not an issue event
	event, ok := hook.Event.(IssueEvent)
	if !ok {
		return nil
	}

	eventAction := event.ObjectAttributes.Action
	// Non-update action
	if eventAction != "update" {
		return nil
	}
	// No labels changed
	if event.Changes.Labels == nil {
		return nil
	}

	projectID := event.ObjectAttributes.ProjectID
	issueID := event.ObjectAttributes.ID
	issueIID := event.ObjectAttributes.IID

	ctx, log := logger.WithFields(ctx, logger.Fields{
		"gitlab_issue_id":   issueID,
		"gitlab_issue_iid":  issueIID,
		"gitlab_project_id": projectID,
	})
	log.Debugf("Processing incoming webhook action %q, labels are changed", eventAction)

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

	var reqID string
	err := a.db.ViewIssues(projectID, func(issues IssuesBucket) error {
		reqID = issues.GetRequestID(issueIID)
		return nil
	})

	ctx, log = logger.WithField(ctx, "request_id", reqID)

	if trace.Unwrap(err) == ErrNoBucket || reqID == "" {
		log.WithError(err).Warning("Failed to find an issue in database")
		reqID = event.ObjectAttributes.ParseDescriptionRequestID()
		if reqID == "" {
			// Ignore the issue, probably it wasn't created by us at all.
			return nil
		}
		log.Warning("Request ID was parsed from issue description")
	} else if err != nil {
		return trace.Wrap(err)
	}

	reqs, err := a.apiClient.GetAccessRequests(ctx, types.AccessRequestFilter{ID: reqID})
	if err != nil {
		return trace.Wrap(err)
	}

	var req types.AccessRequest
	if len(reqs) > 0 {
		req = reqs[0]
	}

	// Validate plugin data that it's matching with the webhook information
	pluginData, err := a.getPluginData(ctx, reqID)
	if err != nil {
		return trace.Wrap(err)
	}
	if pluginData.IssueID == 0 || pluginData.IssueIID == 0 || pluginData.ProjectID == 0 {
		return trace.Errorf("plugin data is blank")
	}
	if pluginData.IssueID != issueID {
		log.WithField("plugin_data_issue_id", pluginData.IssueID).
			Debug("plugin_data.issue_id does not match event.issue_id")
		return trace.Errorf("issue_id from request's plugin_data does not match")
	}
	if pluginData.IssueIID != issueIID {
		log.WithField("plugin_data_issue_iid", pluginData.IssueIID).
			Debug("plugin_data.issue_iid does not match event.issue_iid")
		return trace.Errorf("issue_iid from request's plugin_data does not match")
	}
	if pluginData.ProjectID != projectID {
		log.WithField("plugin_data_project_id", pluginData.ProjectID).
			Debug("plugin_data.project_id does not match event.project_id")
		return trace.Errorf("project_id from request's plugin_data does not match")
	}

	if req == nil {
		return trace.Wrap(a.resolveIssue(ctx, reqID, Resolution{Tag: ResolvedExpired}))
	}

	var resolution Resolution
	state := req.GetState()
	switch {
	case state.IsPending():
		switch action {
		case ApproveAction:
			resolution.Tag = ResolvedApproved
		case DenyAction:
			resolution.Tag = ResolvedDenied
		default:
			return trace.BadParameter("unknown action: %v", action)
		}
		ctx, _ := logger.WithFields(ctx, logger.Fields{
			"gitlab_user_name":     event.User.Name,
			"gitlab_user_username": event.User.Username,
			"gitlab_user_email":    event.User.Email,
		})
		if err := a.resolveRequest(ctx, reqID, event.User.Email, resolution); err != nil {
			return trace.Wrap(err)
		}
	case state.IsApproved():
		resolution.Tag = ResolvedApproved
	case state.IsDenied():
		resolution.Tag = ResolvedDenied
	default:
		return trace.BadParameter("unknown request state %v (%q)", state, state)
	}

	return trace.Wrap(a.resolveIssue(ctx, reqID, resolution))
}

func (a *App) onPendingRequest(ctx context.Context, req types.AccessRequest) error {
	reqID := req.GetName()
	reqData := RequestData{User: req.GetUser(), Roles: req.GetRoles(), Created: req.GetCreationTime()}

	// Create plugin data if it didn't exist before.
	isNew, err := a.modifyPluginData(ctx, reqID, func(existing *PluginData) (PluginData, bool) {
		if existing != nil {
			return PluginData{}, false
		}
		return PluginData{RequestData: reqData}, true
	})
	if err != nil {
		return trace.Wrap(err)
	}

	if isNew {
		if err := a.createIssue(ctx, a.defaultProjectID, reqID, reqData); err != nil {
			return trace.Wrap(err)
		}
	}

	if reqReviews := req.GetReviews(); len(reqReviews) > 0 {
		if err = a.postReviewComments(ctx, reqID, reqReviews); err != nil {
			return trace.Wrap(err)
		}
	}

	return trace.Wrap(err)
}

func (a *App) onResolvedRequest(ctx context.Context, req types.AccessRequest) error {
	err1 := trace.Wrap(a.postReviewComments(ctx, req.GetName(), req.GetReviews()))

	resolution := Resolution{Reason: req.GetResolveReason()}
	switch req.GetState() {
	case types.RequestState_APPROVED:
		resolution.Tag = ResolvedApproved
	case types.RequestState_DENIED:
		resolution.Tag = ResolvedDenied
	}
	err2 := trace.Wrap(a.resolveIssue(ctx, req.GetName(), resolution))

	return trace.NewAggregate(err1, err2)
}

func (a *App) onDeletedRequest(ctx context.Context, reqID string) error {
	return a.resolveIssue(ctx, reqID, Resolution{Tag: ResolvedExpired})
}

// createIssue posts a GitLab issue with request information.
func (a *App) createIssue(ctx context.Context, projectID IntID, reqID string, reqData RequestData) error {
	ctx, _ = logger.WithField(ctx, "gitlab_project_id", projectID)

	data, err := a.gitlab.CreateIssue(ctx, projectID, reqID, reqData)
	if err != nil {
		return trace.Wrap(err)
	}

	issueIID := data.IssueIID

	ctx, log := logger.WithField(ctx, "gitlab_issue_iid", issueIID)
	log.Info("GitLab issue created")

	// Save GitLab issue to request id mapping into file database.
	err1 := a.db.UpdateIssues(data.ProjectID, func(issues IssuesBucket) error {
		return issues.SetRequestID(issueIID, reqID)
	})
	if err1 != nil {
		return trace.Wrap(err1)
	}

	// Save GitLab issue info in plugin data.
	_, err2 := a.modifyPluginData(ctx, reqID, func(existing *PluginData) (PluginData, bool) {
		var pluginData PluginData
		if existing != nil {
			pluginData = *existing
		} else {
			// It must be impossible but lets handle it just in case.
			pluginData = PluginData{RequestData: reqData}
		}
		pluginData.GitlabData = data
		return pluginData, true
	})

	return trace.NewAggregate(err1, err2)
}

// postReviewComments posts issue comments about new reviews appeared for request.
func (a *App) postReviewComments(ctx context.Context, reqID string, reqReviews []types.AccessReview) error {
	var oldCount int
	var data GitlabData

	// Increase the review counter in plugin data.
	ok, err := a.modifyPluginData(ctx, reqID, func(existing *PluginData) (PluginData, bool) {
		// If plugin data is missing issue identification info, we cannot do anything.
		if existing == nil {
			data = GitlabData{}
			return PluginData{}, false
		}

		data = existing.GitlabData
		// If plugin data has blank issue identification info, we cannot do anything.
		if data.ProjectID == 0 || data.IssueIID == 0 {
			return PluginData{}, false
		}

		count := len(reqReviews)
		// If reviews counter is at least the same as it was before, we shouldn't do anything.
		if oldCount = existing.ReviewsCount; oldCount >= count {
			return PluginData{}, false
		}
		pluginData := *existing
		pluginData.ReviewsCount = count
		return pluginData, true
	})
	if err != nil {
		return trace.Wrap(err)
	}
	if !ok {
		if data.ProjectID == 0 || data.IssueIID == 0 {
			logger.Get(ctx).Debug("Failed to post the comment: plugin data is blank")
		}
		return nil
	}
	ctx, _ = logger.WithFields(ctx, logger.Fields{
		"gitlab_project_id": data.ProjectID,
		"gitlab_issue_iid":  data.IssueIID,
	})

	slice := reqReviews[oldCount:]
	if len(slice) == 0 {
		return nil
	}

	errors := make([]error, 0, len(slice))
	for _, review := range slice {
		if err := a.gitlab.PostReviewComment(ctx, data.ProjectID, data.IssueIID, review); err != nil {
			errors = append(errors, err)
		}
	}
	return trace.NewAggregate(errors...)
}

// resolveRequest sets an access request state.
func (a *App) resolveRequest(ctx context.Context, reqID string, userEmail string, resolution Resolution) error {
	params := types.AccessRequestUpdate{RequestID: reqID}

	switch resolution.Tag {
	case ResolvedApproved:
		params.State = types.RequestState_APPROVED
	case ResolvedDenied:
		params.State = types.RequestState_DENIED
	default:
		return trace.BadParameter("unknown resolution tag %v", resolution.Tag)
	}

	delegator := fmt.Sprintf("%s:%s", pluginName, userEmail)

	if err := a.apiClient.SetAccessRequestState(apiutils.WithDelegator(ctx, delegator), params); err != nil {
		return trace.Wrap(err)
	}

	logger.Get(ctx).Infof("GitLab user %s the request", resolution.Tag)
	return nil
}

// resolveIssue closes the issue to some final state.
func (a *App) resolveIssue(ctx context.Context, reqID string, resolution Resolution) error {
	var data GitlabData

	// Save request resolution info in plugin data.
	ok, err := a.modifyPluginData(ctx, reqID, func(existing *PluginData) (PluginData, bool) {
		// If plugin data is missing issue identification info, we cannot do anything.
		if existing == nil {
			data = GitlabData{}
			return PluginData{}, false
		}

		data = existing.GitlabData
		// If plugin data has blank issue identification info, we cannot do anything.
		if data.ProjectID == 0 || data.IssueIID == 0 {
			return PluginData{}, false
		}

		// If resolution field is not empty then we already resolved the issue before. In this case we just quit.
		if existing.RequestData.Resolution.Tag != Unresolved {
			return PluginData{}, false
		}

		// Mark issue as resolved.
		pluginData := *existing
		pluginData.Resolution = resolution
		return pluginData, true
	})
	if err != nil {
		return trace.Wrap(err)
	}
	if !ok {
		if data.ProjectID == 0 || data.IssueIID == 0 {
			logger.Get(ctx).Debug("Failed to resolve the issue: plugin data is blank")
		} else {
			logger.Get(ctx).Debug("Issue was already resolved by us")
		}

		// Either plugin data is missing or issue is already resolved by us, just quit.
		return nil
	}

	ctx, log := logger.WithFields(ctx, logger.Fields{
		"gitlab_project_id": data.ProjectID,
		"gitlab_issue_iid":  data.IssueIID,
	})
	if err := a.gitlab.ResolveIssue(ctx, data.ProjectID, data.IssueIID, resolution); err != nil {
		return trace.Wrap(err)
	}
	log.Info("Successfully resolved the issue")

	return nil
}

// plugindata makes a plugindata client.
func (a *App) plugindata() plugindata.Client {
	return plugindata.Client{
		APIClient:  a.apiClient,
		PluginName: pluginName,
	}
}

// modifyPluginData performs a compare-and-swap update of access request's plugin data.
// For details, see the plugindata package documentation on the Modify method.
func (a *App) modifyPluginData(ctx context.Context, reqID string, fn func(*PluginData) (PluginData, bool)) (bool, error) {
	ok, err := a.plugindata().Modify(
		ctx,
		backoff.NewDecorr(modifyPluginDataBackoffBase, modifyPluginDataBackoffMax, clockwork.NewRealClock()),
		types.KindAccessRequest,
		reqID,
		&PluginData{},
		func(data interface{}) (plugindata.Marshaller, bool) {
			newData, ok := fn(data.(*PluginData))
			return &newData, ok
		},
	)
	return ok, trace.Wrap(err)
}

// getPluginData loads a plugin data for a given access request. It returns nil if it's not found.
func (a *App) getPluginData(ctx context.Context, reqID string) (PluginData, error) {
	var data PluginData
	if err := a.plugindata().Get(ctx, types.KindAccessRequest, reqID, &data); err != nil {
		return PluginData{}, trace.Wrap(err)
	}
	return data, nil
}
