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
	"strings"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport-plugins/lib/watcherjob"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"

	"github.com/gravitational/trace"
)

const (
	// minServerVersion is the minimal teleport version the plugin supports.
	minServerVersion = "6.1.0"
	// pluginName is used to tag PluginData and as a Delegator in Audit log.
	pluginName = "pagerduty"
	// backoffMaxDelay is a maximum time GRPC client waits before reconnection attempt.
	backoffMaxDelay = time.Second * 2
	// initTimeout is used to bound execution time of health check and teleport version check.
	initTimeout = time.Second * 10
	// handlerTimeout is used to bound the execution time of watcher event handler.
	handlerTimeout = time.Second * 5
	// maxModifyPluginDataTries is a maximum number of compare-and-swap tries when modifying plugin data.
	maxModifyPluginDataTries = 5
)

// App contains global application state.
type App struct {
	conf Config

	apiClient *client.Client
	pagerduty Pagerduty
	mainJob   lib.ServiceJob

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
	var err error

	log := logger.Get(ctx)
	log.Infof("Starting Teleport Access PagerDuty Plugin %s:%s", Version, Gitref)

	if err = a.init(ctx); err != nil {
		return trace.Wrap(err)
	}

	watcherJob := watcherjob.NewJob(
		a.apiClient,
		watcherjob.Config{
			Watch:            types.Watch{Kinds: []types.WatchKind{types.WatchKind{Kind: types.KindAccessRequest}}},
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

	bk := backoff.DefaultConfig
	bk.MaxDelay = backoffMaxDelay
	if a.apiClient, err = client.New(ctx, client.Config{
		Addrs:       []string{a.conf.Teleport.AuthServer},
		Credentials: a.conf.Teleport.Credentials(),
		DialOpts:    []grpc.DialOption{grpc.WithConnectParams(grpc.ConnectParams{Backoff: bk, MinConnectTimeout: initTimeout})},
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
	a.pagerduty, err = NewPagerdutyClient(a.conf.Pagerduty, pong.ClusterName, webProxyAddr)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Debug("Starting PagerDuty API health check...")
	if err = a.pagerduty.HealthCheck(ctx); err != nil {
		return trace.Wrap(err, "api health check failed. check your credentials and service_id settings")
	}
	log.Debug("PagerDuty API health check finished ok")

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
			log.WithError(err).Error("Failed to process deleted request")
			return trace.Wrap(err)
		}
		return nil
	default:
		return trace.BadParameter("unexpected event operation %s", op)
	}
}

func (a *App) onPendingRequest(ctx context.Context, req types.AccessRequest) error {
	if len(req.GetSystemAnnotations()) == 0 {
		logger.Get(ctx).Debug("Cannot proceed further. Request is missing any annotations")
		return nil
	}

	var (
		resultErr error
		data      PagerdutyData
	)

	shouldTryApprove := true

	// First, try to create a notification incident.
	if serviceName, err := a.getNotifyServiceName(req); err == nil {
		var isNew bool
		if data, isNew, err = a.tryNotifyService(ctx, req, serviceName); err == nil {
			// To minimize the count of auto-approval tries, lets attempt it only when we just created an incident.
			shouldTryApprove = isNew
		} else {
			resultErr = trace.Wrap(err)
			// If there's an error, we can't really know is the incident new or not so lets just try.
			shouldTryApprove = true
		}
	} else {
		logger.Get(ctx).Debugf("Failed to determine a notification service info: %s", err.Error())
	}

	if !shouldTryApprove {
		return resultErr
	}

	// Then, try to approve the request if user is currently on-call.
	err := a.tryApproveRequest(ctx, req, data.IncidentID)
	return trace.NewAggregate(resultErr, trace.Wrap(err))
}

func (a *App) onResolvedRequest(ctx context.Context, req types.AccessRequest) error {
	var notifyErr error
	if _, err := a.postReviewNotes(ctx, req.GetName(), req.GetReviews()); err != nil {
		notifyErr = trace.Wrap(err)
	}

	resolution := Resolution{Reason: req.GetResolveReason()}
	switch req.GetState() {
	case types.RequestState_APPROVED:
		resolution.Tag = ResolvedApproved
	case types.RequestState_DENIED:
		resolution.Tag = ResolvedDenied
	}
	err := trace.Wrap(a.resolveIncident(ctx, req.GetName(), resolution))
	return trace.NewAggregate(notifyErr, err)
}

func (a *App) onDeletedRequest(ctx context.Context, reqID string) error {
	return a.resolveIncident(ctx, reqID, Resolution{Tag: ResolvedExpired})
}

func (a *App) getNotifyServiceName(req types.AccessRequest) (string, error) {
	annotationKey := a.conf.Pagerduty.RequestAnnotations.NotifyService
	slice, ok := req.GetSystemAnnotations()[annotationKey]
	if !ok {
		return "", trace.Errorf("request annotation %q is missing", annotationKey)
	}
	var serviceName string
	if len(slice) > 0 {
		serviceName = slice[0]
	}
	if serviceName == "" {
		return "", trace.Errorf("request annotation %q is empty", annotationKey)
	}
	return serviceName, nil
}

func (a *App) tryNotifyService(ctx context.Context, req types.AccessRequest, serviceName string) (PagerdutyData, bool, error) {
	ctx, _ = logger.WithField(ctx, "pd_service_name", serviceName)
	service, err := a.pagerduty.FindServiceByName(ctx, serviceName)
	if err != nil {
		return PagerdutyData{}, false, trace.Wrap(err)
	}

	reqID := req.GetName()
	reqData := RequestData{
		User:          req.GetUser(),
		Roles:         req.GetRoles(),
		Created:       req.GetCreationTime(),
		RequestReason: req.GetRequestReason(),
	}

	// Create plugin data if it didn't exist before.
	isNew, err := a.modifyPluginData(ctx, reqID, func(existing *PluginData) (PluginData, bool) {
		if existing != nil {
			return PluginData{}, false
		}
		return PluginData{RequestData: reqData}, true
	})
	if err != nil {
		return PagerdutyData{}, isNew, trace.Wrap(err)
	}

	var data PagerdutyData
	if isNew {
		if data, err = a.createIncident(ctx, service.ID, reqID, reqData); err != nil {
			return data, isNew, trace.Wrap(err)
		}
	}

	if reqReviews := req.GetReviews(); len(reqReviews) > 0 {
		if data, err = a.postReviewNotes(ctx, reqID, reqReviews); err != nil {
			return data, isNew, trace.Wrap(err)
		}
	}

	return data, isNew, nil
}

// createIncident posts an incident with request information.
func (a *App) createIncident(ctx context.Context, serviceID, reqID string, reqData RequestData) (PagerdutyData, error) {
	data, err := a.pagerduty.CreateIncident(ctx, serviceID, reqID, reqData)
	if err != nil {
		return PagerdutyData{}, trace.Wrap(err)
	}
	ctx, log := logger.WithField(ctx, "pd_incident_id", data.IncidentID)
	log.Info("Successfully created PagerDuty incident")

	// Save pagerduty incident info in plugin data.
	_, err = a.modifyPluginData(ctx, reqID, func(existing *PluginData) (PluginData, bool) {
		var pluginData PluginData
		if existing != nil {
			pluginData = *existing
		} else {
			// It must be impossible but lets handle it just in case.
			pluginData = PluginData{RequestData: reqData}
		}
		pluginData.PagerdutyData = data
		return pluginData, true
	})
	return data, trace.Wrap(err)
}

// postReviewNotes posts incident notes about new reviews appeared for request.
func (a *App) postReviewNotes(ctx context.Context, reqID string, reqReviews []types.AccessReview) (PagerdutyData, error) {
	var oldCount int
	var data PagerdutyData

	// Increase the review counter in plugin data.
	ok, err := a.modifyPluginData(ctx, reqID, func(existing *PluginData) (PluginData, bool) {
		if existing == nil {
			return PluginData{}, false
		}

		if data = existing.PagerdutyData; data.IncidentID == "" {
			return PluginData{}, false
		}

		count := len(reqReviews)
		if oldCount = existing.ReviewsCount; oldCount >= count {
			return PluginData{}, false
		}
		pluginData := *existing
		pluginData.ReviewsCount = count
		return pluginData, true
	})
	if err != nil {
		return data, trace.Wrap(err)
	}
	if !ok {
		logger.Get(ctx).Debug("Failed to post the note: plugin data is missing")
		return data, nil
	}
	ctx, _ = logger.WithField(ctx, "pd_incident_id", data.IncidentID)

	slice := reqReviews[oldCount:]
	if len(slice) == 0 {
		return data, nil
	}

	errors := make([]error, 0, len(slice))
	for _, review := range slice {
		if err := a.pagerduty.PostReviewNote(ctx, data.IncidentID, review); err != nil {
			errors = append(errors, err)
		}
	}
	return data, trace.NewAggregate(errors...)
}

// tryApproveRequest attempts to submit an approval if the following conditions are met:
//   1. Requesting user must be on-call in one of the services provided in request annotation.
//   2. User must have an active incident in such service.
func (a *App) tryApproveRequest(ctx context.Context, req types.AccessRequest, notifyServiceID string) error {
	log := logger.Get(ctx)

	annotationKey := a.conf.Pagerduty.RequestAnnotations.Services
	serviceNames, ok := req.GetSystemAnnotations()[annotationKey]
	if !ok {
		logger.Get(ctx).Debugf("Failed to submit approval: request annotation %q is missing", annotationKey)
		return nil
	}
	if len(serviceNames) == 0 {
		log.Warningf("Failed to find any service name: request annotation %q is empty", annotationKey)
		return nil
	}

	userName := req.GetUser()
	if !lib.IsEmail(userName) {
		logger.Get(ctx).Warningf("Failed to submit approval: %q does not look like a valid email", userName)
		return nil
	}

	user, err := a.pagerduty.FindUserByEmail(ctx, userName)
	if err != nil {
		if trace.IsNotFound(err) {
			log.WithError(err).Debugf("Failed to submit approval: %q email is not found", userName)
			return nil
		}
		return trace.Wrap(err)
	}

	ctx, log = logger.WithFields(ctx, logger.Fields{
		"pd_user_email": user.Email,
		"pd_user_name":  user.Name,
	})

	services, err := a.pagerduty.FindServicesByNames(ctx, serviceNames)
	if err != nil {
		return trace.Wrap(err)
	}
	if len(services) == 0 {
		log.WithField("pd_service_names", serviceNames).Warning("Failed to find any service")
		return nil
	}

	if notifyServiceID != "" {
		filteredServices := make([]Service, 0, len(services))
		for _, service := range services {
			if service.ID == notifyServiceID {
				log.WithField("pd_service_name", service.Name).Warn("Notification service and approval services should not overlap")
				continue
			}
			filteredServices = append(filteredServices, service)
		}
		services = filteredServices
		if len(services) == 0 {
			return nil
		}
	}

	escalationPolicyMapping := make(map[string][]Service)
	for _, service := range services {
		escalationPolicyMapping[service.EscalationPolicy.ID] = append(escalationPolicyMapping[service.EscalationPolicy.ID], service)
	}
	var escalationPolicyIDs []string
	for id := range escalationPolicyMapping {
		escalationPolicyIDs = append(escalationPolicyIDs, id)
	}

	if escalationPolicyIDs, err = a.pagerduty.FilterOnCallPolicies(ctx, user.ID, escalationPolicyIDs); err != nil {
		return trace.Wrap(err)
	}
	if len(escalationPolicyIDs) == 0 {
		log.Debug("Failed to submit approval: user is not on call")
		return nil
	}

	serviceNames = make([]string, 0, len(services))
	serviceIDs := make([]string, 0, len(services))
	for _, policyID := range escalationPolicyIDs {
		for _, service := range escalationPolicyMapping[policyID] {
			serviceIDs = append(serviceIDs, service.ID)
			serviceNames = append(serviceNames, service.Name)
		}
	}
	if len(serviceIDs) == 0 {
		return nil
	}

	ok, err = a.pagerduty.HasAssignedIncidents(ctx, user.ID, serviceIDs)
	if err != nil {
		return trace.Wrap(err)
	}
	if !ok {
		log.Debug("Failed to submit approval: user has no incidents assigned")
		return nil
	}

	if _, err := a.apiClient.SubmitAccessReview(ctx, types.AccessReviewSubmission{
		RequestID: req.GetName(),
		Review: types.AccessReview{
			ProposedState: types.RequestState_APPROVED,
			Reason: fmt.Sprintf("Access requested by user %s (%s) which is on call in service(s) %s and has some active incidents assigned",
				user.Name,
				user.Email,
				strings.Join(serviceNames, ","),
			),
			Created: time.Now(),
		},
	}); err != nil {
		if strings.HasSuffix(err.Error(), "has already reviewed this request") {
			log.Debug("Already reviewed the request")
			return nil
		}
		return trace.Wrap(err)
	}

	log.Info("Successfully submitted a request approval")
	return nil
}

// resolveIncident resolves the notification incident created by plugin if the incident exists.
func (a *App) resolveIncident(ctx context.Context, reqID string, resolution Resolution) error {
	var incidentID string

	// Save request resolution info in plugin data.
	ok, err := a.modifyPluginData(ctx, reqID, func(existing *PluginData) (PluginData, bool) {
		// If plugin data is empty or missing incidentID, we cannot do anything.
		if existing == nil {
			return PluginData{}, false
		}
		if incidentID = existing.IncidentID; incidentID == "" {
			return PluginData{}, false
		}

		// If resolution field is not empty then we already resolved the incident before. In this case we just quit.
		if existing.RequestData.Resolution.Tag != Unresolved {
			return PluginData{}, false
		}

		// Mark incident as resolved.
		pluginData := *existing
		pluginData.Resolution = resolution
		return pluginData, true
	})
	if err != nil {
		return trace.Wrap(err)
	}
	if !ok {
		logger.Get(ctx).Debug("Failed to resolve the incident: plugin data is missing")
		return nil
	}

	ctx, log := logger.WithField(ctx, "pd_incident_id", incidentID)
	if err := a.pagerduty.ResolveIncident(ctx, incidentID, resolution); err != nil {
		return trace.Wrap(err)
	}
	log.Info("Successfully resolved the incident")

	return nil
}

// modifyPluginData performs a compare-and-swap update of access request's plugin data.
func (a *App) modifyPluginData(ctx context.Context, reqID string, fn func(data *PluginData) (PluginData, bool)) (bool, error) {
	var lastErr error
	for i := 0; i < maxModifyPluginDataTries; i++ {
		oldData, err := a.getPluginData(ctx, reqID)
		if err != nil && !trace.IsNotFound(err) {
			return false, trace.Wrap(err)
		}
		newData, ok := fn(oldData)
		if !ok {
			return false, nil
		}
		var expectData PluginData
		if oldData != nil {
			expectData = *oldData
		}
		err = trace.Wrap(a.updatePluginData(ctx, reqID, newData, expectData))
		if err == nil {
			return true, nil
		}
		if trace.IsCompareFailed(err) {
			lastErr = err
			continue
		}
		return false, err
	}
	return false, lastErr
}

// getPluginData loads a plugin data for a given access request. It returns nil if it's not found.
func (a *App) getPluginData(ctx context.Context, reqID string) (*PluginData, error) {
	dataMaps, err := a.apiClient.GetPluginData(ctx, types.PluginDataFilter{
		Kind:     types.KindAccessRequest,
		Resource: reqID,
		Plugin:   pluginName,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if len(dataMaps) == 0 {
		return nil, trace.NotFound("plugin data not found")
	}
	entry := dataMaps[0].Entries()[pluginName]
	if entry == nil {
		return nil, trace.NotFound("plugin data entry not found")
	}
	data := DecodePluginData(entry.Data)
	return &data, nil
}

// updatePluginData updates an existing plugin data or sets a new one if it didn't exist.
func (a *App) updatePluginData(ctx context.Context, reqID string, data PluginData, expectData PluginData) error {
	return a.apiClient.UpdatePluginData(ctx, types.PluginDataUpdateParams{
		Kind:     types.KindAccessRequest,
		Resource: reqID,
		Plugin:   pluginName,
		Set:      EncodePluginData(data),
		Expect:   EncodePluginData(expectData),
	})
}
