/*
Copyright 2021 Gravitational, Inc.

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

package integration

import (
	"context"
	"sync"
	"time"

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/trace"
)

const APIUsername = "integration-api"

type ClientFactory interface {
	Client(ctx context.Context, service Service, userName string) (*client.Client, error)
}

type API struct {
	mu            sync.Mutex
	service       Service
	factory       ClientFactory
	impersonation map[string]*client.Client
}

func newAPI(ctx context.Context, service Service, factory ClientFactory) (*API, error) {
	api := API{
		service:       service,
		factory:       factory,
		impersonation: make(map[string]*client.Client),
	}
	client, err := factory.Client(ctx, service, APIUsername)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	api.impersonation[""] = client // set default client
	return &api, nil
}

func (api *API) impersonate(ctx context.Context, userName string) (*client.Client, error) {
	api.mu.Lock()
	client, ok := api.impersonation[userName]
	api.mu.Unlock()

	if ok {
		return client, nil
	}

	client, err := api.factory.Client(ctx, api.service, userName)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	api.mu.Lock()
	api.impersonation[userName] = client
	api.mu.Unlock()

	return client, nil
}

// Ping gets basic info about the auth server.
func (api *API) Ping(ctx context.Context) (proto.PingResponse, error) {
	authClient, err := api.impersonate(ctx, "")
	if err != nil {
		return proto.PingResponse{}, trace.Wrap(err)
	}
	return authClient.Ping(ctx)
}

// UpsertRole creates or updates role.
func (api *API) UpsertRole(ctx context.Context, role types.Role) error {
	authClient, err := api.impersonate(ctx, "")
	if err != nil {
		return trace.Wrap(err)
	}
	return authClient.UpsertRole(ctx, role)
}

// CreateUser creates a new user from the specified descriptor.
func (api *API) CreateUser(ctx context.Context, user types.User) error {
	authClient, err := api.impersonate(ctx, "")
	if err != nil {
		return trace.Wrap(err)
	}
	return authClient.CreateUser(ctx, user)
}

// CreateUserWithRoles is a helper method for creating a user with a given set of roles.
func (api *API) CreateUserWithRoles(ctx context.Context, name string, roles ...string) (types.User, error) {
	user, err := types.NewUser(name)
	if err != nil {
		return user, trace.Wrap(err)
	}
	user.SetRoles(roles)
	if err := api.CreateUser(ctx, user); err != nil {
		return nil, trace.Wrap(err)
	}
	return user, nil
}

// CreateAccessRequest registers a new access request with the auth server. Request is being sent on behalf of request user.
func (api *API) CreateAccessRequest(ctx context.Context, req types.AccessRequest) error {
	authClient, err := api.impersonate(ctx, req.GetUser())
	if err != nil {
		return trace.Wrap(err)
	}
	return authClient.CreateAccessRequest(ctx, req)
}

// SetAccessRequestState updates the state of an existing access request.
func (api *API) SetAccessRequestState(ctx context.Context, update types.AccessRequestUpdate) error {
	authClient, err := api.impersonate(ctx, "")
	if err != nil {
		return trace.Wrap(err)
	}
	return authClient.SetAccessRequestState(ctx, update)
}

// ApproveAccessRequest sets an access request state to APPROVED.
func (api *API) ApproveAccessRequest(ctx context.Context, reqID, reason string) error {
	update := types.AccessRequestUpdate{
		RequestID: reqID,
		State:     types.RequestState_APPROVED,
		Reason:    reason,
	}
	return api.SetAccessRequestState(ctx, update)
}

// ApproveAccessRequest sets an access request state to DENIED.
func (api *API) DenyAccessRequest(ctx context.Context, reqID, reason string) error {
	update := types.AccessRequestUpdate{
		RequestID: reqID,
		State:     types.RequestState_DENIED,
		Reason:    reason,
	}
	return api.SetAccessRequestState(ctx, update)
}

// DeleteAccessRequest deletes an access request.
func (api *API) DeleteAccessRequest(ctx context.Context, reqID string) error {
	authClient, err := api.impersonate(ctx, "")
	if err != nil {
		return trace.Wrap(err)
	}
	return authClient.DeleteAccessRequest(ctx, reqID)
}

// GetAccessRequest loads an access request.
func (api *API) GetAccessRequest(ctx context.Context, reqID string) (types.AccessRequest, error) {
	authClient, err := api.impersonate(ctx, "")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	requests, err := authClient.GetAccessRequests(ctx, types.AccessRequestFilter{ID: reqID})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if len(requests) == 0 {
		return nil, trace.NotFound("request %q is not found", reqID)
	}
	return requests[0], nil
}

// PollAccessRequestPluginData waits until plugin data for a give request became available.
func (api *API) PollAccessRequestPluginData(ctx context.Context, plugin, reqID string) (map[string]string, error) {
	authClient, err := api.impersonate(ctx, "")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	filter := types.PluginDataFilter{
		Kind:     types.KindAccessRequest,
		Resource: reqID,
		Plugin:   plugin,
	}
	for {
		pluginDatas, err := authClient.GetPluginData(ctx, filter)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if len(pluginDatas) > 0 {
			pluginData := pluginDatas[0]
			entry := pluginData.Entries()[plugin]
			if entry != nil {
				return entry.Data, nil
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// SubmitAccessReview applies a review to a request and returns the post-application state. Application is being sent on behalf of review author.
func (api *API) SubmitAccessReview(ctx context.Context, reqID string, review types.AccessReview) (types.AccessRequest, error) {
	authClient, err := api.impersonate(ctx, review.Author)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return authClient.SubmitAccessReview(ctx, types.AccessReviewSubmission{
		RequestID: reqID,
		Review:    review,
	})
}

// SearchAccessRequestEvents searches for recent access request events in audit log.
func (api *API) SearchAccessRequestEvents(ctx context.Context, reqID string) ([]*events.AccessRequestCreate, error) {
	authClient, err := api.impersonate(ctx, "")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	auditEvents, _, err := authClient.SearchEvents(
		ctx,
		time.Now().UTC().AddDate(0, -1, 0),
		time.Now().UTC(),
		"default",
		[]string{"access_request.update"},
		100,
		types.EventOrderAscending,
		"",
	)
	result := make([]*events.AccessRequestCreate, 0, len(auditEvents))
	for _, event := range auditEvents {
		if event, ok := event.(*events.AccessRequestCreate); ok && event.RequestID == reqID {
			result = append(result, event)
		}
	}
	return result, trace.Wrap(err)
}

// NewWatcher returns a new events watcher.
func (api *API) NewWatcher(ctx context.Context, watch types.Watch) (types.Watcher, error) {
	authClient, err := api.impersonate(ctx, "")
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return authClient.NewWatcher(ctx, watch)
}
