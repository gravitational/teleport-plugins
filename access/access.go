/*
Copyright 2019 Gravitational, Inc.

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

package access

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/google/uuid"
	"github.com/hashicorp/go-version"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

// State represents the state of an access request.
type State = types.RequestState

// StatePending is the state of a pending request.
const StatePending State = types.RequestState_PENDING

// StateApproved is the state of an approved request.
const StateApproved State = types.RequestState_APPROVED

// StateDenied is the state of a denied request.
const StateDenied State = types.RequestState_DENIED

// Op describes the operation type of an event.
type Op = proto.Operation

// OpInit is sent as the first sentinel value on the watch channel.
const OpInit = proto.Operation_INIT

// OpPut inicates creation or update.
const OpPut = proto.Operation_PUT

// OpDelete indicates deletion or expiry.
const OpDelete = proto.Operation_DELETE

type DialOption = grpc.DialOption
type CallOption = grpc.CallOption

// Filter encodes request filtering parameters.
type Filter = types.AccessRequestFilter

// Features contains flags of features supported by auth server.
type Features = proto.Features

// Event is a request event.
type Event struct {
	// Type is the operation type of the event.
	Type Op
	// Request is the payload of the event.
	// NOTE: If Type is OpDelete, only the ID field
	// will be filled.
	Request Request
}

// Request describes a pending access request.
type Request struct {
	// ID is the unique identifier of the request.
	ID string
	// User is the user to whom the request applies.
	User string
	// Roles are the roles that the user will be granted
	// if the request is approved.
	Roles []string
	// State is the current state of the request.
	State State
	// Created is a creation time of the request.
	Created time.Time
	// RequestReason is an optional message explaining the reason for the request.
	RequestReason string
	// ResolveReason is an optional message explaining the reason for the resolution
	// (approval/denail) of the request.
	ResolveReason string
	// ResolveAnnotations is a set of arbitrary values sent by plugins or other
	// resolving parties during approval/denial.
	ResolveAnnotations map[string][]string
	// SystemAnnotations is a set of programmatically generated annotations attached
	// to pending access requests by teleport.
	SystemAnnotations map[string][]string
	// SuggestedReviewers is a set of usernames which are subjects to review the request.
	SuggestedReviewers []string
}

type RequestStateParams struct {
	State       State
	Delegator   string
	Reason      string
	Annotations map[string][]string
}

// Pong describes a ping response.
type Pong struct {
	ServerVersion   string
	ClusterName     string
	ProxyPublicAddr string
	ServerFeatures  *Features
}

// PluginDataMap is a custom user data associated with access request.
type PluginDataMap map[string]string

// Watcher is used to monitor access requests.
type Watcher interface {
	WaitInit(ctx context.Context, timeout time.Duration) error
	Events() <-chan Event
	Done() <-chan struct{}
	Error() error
	Close()
}

// Client is an access request management client.
type Client interface {
	WithCallOptions(...CallOption) Client
	// Ping loads basic information about Teleport version and cluster name
	Ping(ctx context.Context) (Pong, error)
	// WatchRequests registers a new watcher for pending access requests.
	WatchRequests(ctx context.Context, fltr Filter) Watcher
	// CreateRequest creates a request.
	CreateRequest(ctx context.Context, user string, roles ...string) (Request, error)
	// GetRequests loads all requests which match provided filter.
	GetRequests(ctx context.Context, fltr Filter) ([]Request, error)
	// GetRequest loads a request matching ID.
	GetRequest(ctx context.Context, reqID string) (Request, error)
	// SetRequestState updates the state of a request.
	SetRequestState(ctx context.Context, reqID string, state State, delegator string) error
	// SetRequestStateExt is an advanced version of SetRequestState which
	// supports extra features like overriding the requet's role list and
	// attaching annotations (requires teleport v4.4.4 or later).
	SetRequestStateExt(ctx context.Context, reqID string, params RequestStateParams) error
	// GetPluginData fetches plugin data of the specific request.
	GetPluginData(ctx context.Context, reqID string) (PluginDataMap, error)
	// UpdatePluginData updates plugin data of the specific request comparing it with a previous value.
	UpdatePluginData(ctx context.Context, reqID string, set PluginDataMap, expect PluginDataMap) error
}

// clt is a thin wrapper around the raw GRPC types that implements the
// access.Client interface.
type clt struct {
	plugin   string
	clt      proto.AuthServiceClient
	cancel   context.CancelFunc
	callOpts []grpc.CallOption
}

// NewClient creates a new Teleport GRPC API client and returns it.
func NewClient(ctx context.Context, plugin string, addr string, tc *tls.Config, dialOptions ...DialOption) (Client, error) {
	dialOptions = append(dialOptions, grpc.WithTransportCredentials(credentials.NewTLS(tc)))
	ctx, cancel := context.WithCancel(ctx)
	conn, err := grpc.DialContext(ctx, addr, dialOptions...)
	if err != nil {
		cancel()
		return nil, lib.FromGRPC(err)
	}
	authClient := proto.NewAuthServiceClient(conn)
	return &clt{
		plugin: plugin,
		clt:    authClient,
		cancel: cancel,
	}, nil
}

func (c *clt) WithCallOptions(options ...CallOption) Client {
	newClient := *c
	newClient.callOpts = append(newClient.callOpts, options...)
	return &newClient
}

func (c *clt) Ping(ctx context.Context) (Pong, error) {
	rsp, err := c.clt.Ping(ctx, &proto.PingRequest{}, c.callOpts...)
	if err != nil {
		return Pong{}, lib.FromGRPC(err)
	}
	return Pong{
		ServerVersion:   rsp.ServerVersion,
		ClusterName:     rsp.ClusterName,
		ProxyPublicAddr: rsp.ProxyPublicAddr,
		ServerFeatures:  rsp.ServerFeatures,
	}, nil
}

func (c *clt) WatchRequests(ctx context.Context, fltr Filter) Watcher {
	return newWatcher(ctx, c.clt, c.callOpts, fltr)
}

func (c *clt) GetRequests(ctx context.Context, fltr Filter) ([]Request, error) {
	rsp, err := c.clt.GetAccessRequests(ctx, &fltr, c.callOpts...)
	if err != nil {
		return nil, lib.FromGRPC(err)
	}
	var reqs []Request
	for _, req := range rsp.AccessRequests {
		reqs = append(reqs, requestFromV3(req))
	}
	return reqs, nil
}

func (c *clt) CreateRequest(ctx context.Context, user string, roles ...string) (Request, error) {
	req := &types.AccessRequestV3{
		Kind:    types.KindAccessRequest,
		Version: types.V3,
		Metadata: types.Metadata{
			Name: uuid.New().String(),
		},
		Spec: types.AccessRequestSpecV3{
			User:  user,
			Roles: roles,
			State: types.RequestState_PENDING,
		},
	}
	_, err := c.clt.CreateAccessRequest(ctx, req)
	return requestFromV3(req), trace.Wrap(err)
}

func (c *clt) GetRequest(ctx context.Context, reqID string) (Request, error) {
	reqs, err := c.GetRequests(ctx, Filter{
		ID: reqID,
	})
	if err != nil {
		return Request{ID: reqID}, trace.Wrap(err)
	}
	if len(reqs) < 1 {
		return Request{ID: reqID}, trace.NotFound("no request matching %q", reqID)
	}
	return reqs[0], nil
}

func (c *clt) SetRequestState(ctx context.Context, reqID string, state State, delegator string) error {
	_, err := c.clt.SetAccessRequestState(ctx, &proto.RequestStateSetter{
		ID:        reqID,
		State:     state,
		Delegator: fmt.Sprintf("%s:%s", c.plugin, delegator),
	})
	return lib.FromGRPC(err)
}

func (c *clt) SetRequestStateExt(ctx context.Context, reqID string, params RequestStateParams) error {
	_, err := c.clt.SetAccessRequestState(ctx, &proto.RequestStateSetter{
		ID:        reqID,
		State:     params.State,
		Delegator: fmt.Sprintf("%s:%s", c.plugin, params.Delegator),
		Reason:    params.Reason,
	})
	return lib.FromGRPC(err)
}

func (c *clt) GetPluginData(ctx context.Context, reqID string) (PluginDataMap, error) {
	dataSeq, err := c.clt.GetPluginData(ctx, &types.PluginDataFilter{
		Kind:     types.KindAccessRequest,
		Resource: reqID,
		Plugin:   c.plugin,
	})
	if err != nil {
		return nil, lib.FromGRPC(err)
	}
	pluginDatas := dataSeq.GetPluginData()
	if len(pluginDatas) == 0 {
		return PluginDataMap{}, nil
	}

	var pluginData types.PluginData = pluginDatas[0]
	entry := pluginData.Entries()[c.plugin]
	if entry == nil {
		return PluginDataMap{}, nil
	}
	return entry.Data, nil
}

func (c *clt) UpdatePluginData(ctx context.Context, reqID string, set PluginDataMap, expect PluginDataMap) (err error) {
	_, err = c.clt.UpdatePluginData(ctx, &types.PluginDataUpdateParams{
		Kind:     types.KindAccessRequest,
		Resource: reqID,
		Plugin:   c.plugin,
		Set:      set,
		Expect:   expect,
	})
	return lib.FromGRPC(err)
}

func (c *clt) Close() {
	c.cancel()
}

type watcher struct {
	eventC chan Event
	initC  chan struct{}
	doneC  chan struct{}
	emux   sync.Mutex
	err    error
	cancel context.CancelFunc
}

func newWatcher(ctx context.Context, clt proto.AuthServiceClient, callOpts []CallOption, fltr Filter) *watcher {
	ctx, cancel := context.WithCancel(ctx)
	w := &watcher{
		eventC: make(chan Event),
		initC:  make(chan struct{}),
		doneC:  make(chan struct{}),
		cancel: cancel,
	}
	go w.run(ctx, clt, callOpts, fltr)
	return w
}

func (w *watcher) run(ctx context.Context, clt proto.AuthServiceClient, callOpts []CallOption, fltr Filter) {
	defer w.Close()
	defer close(w.doneC)

	stream, err := clt.WatchEvents(ctx, &proto.Watch{
		Kinds: []proto.WatchKind{
			proto.WatchKind{
				Kind:   types.KindAccessRequest,
				Filter: fltr.IntoMap(),
			},
		},
	}, callOpts...)
	if err != nil {
		w.setError(lib.FromGRPC(err))
		return
	}

	for {
		event, err := stream.Recv()
		if err != nil {
			w.setError(lib.FromGRPC(err))
			return
		}
		var req Request
		switch event.Type {
		case OpInit:
			close(w.initC)
			continue
		case OpPut:
			r := event.GetAccessRequest()
			if r == nil {
				w.setError(trace.Errorf("unexpected resource type %T", event.Resource))
				return
			}
			req = requestFromV3(r)
		case OpDelete:
			h := event.GetResourceHeader()
			if h == nil {
				w.setError(trace.Errorf("expected resource header, got %T", event.Resource))
				return
			}
			req = Request{
				ID: h.Metadata.Name,
			}
		default:
			w.setError(trace.Errorf("unexpected event op type %s", event.Type))
			return
		}
		w.eventC <- Event{
			Type:    event.Type,
			Request: req,
		}
	}
}

func (w *watcher) WaitInit(ctx context.Context, timeout time.Duration) error {
	select {
	case <-w.initC:
		return nil
	case <-time.After(timeout):
		return trace.ConnectionProblem(nil, "watcher initialization timed out")
	case <-w.Done():
		return w.Error()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *watcher) Events() <-chan Event {
	return w.eventC
}

func (w *watcher) Done() <-chan struct{} {
	return w.doneC
}

func (w *watcher) Error() error {
	w.emux.Lock()
	defer w.emux.Unlock()
	return w.err
}

func (w *watcher) setError(err error) {
	w.emux.Lock()
	defer w.emux.Unlock()
	w.err = err
}

func (w *watcher) Close() {
	w.cancel()
}

// AssertServerVersion returns an error if server version in ping response is
// less than minimum required version.
func (p Pong) AssertServerVersion(minVersion string) error {
	actual, err := version.NewVersion(p.ServerVersion)
	if err != nil {
		return trace.Wrap(err)
	}
	required, err := version.NewVersion(minVersion)
	if err != nil {
		return trace.Wrap(err)
	}
	if actual.LessThan(required) {
		return trace.Errorf("server version %s is less than %s", p.ServerVersion, minVersion)
	}
	return nil
}

func requestFromV3(req *types.AccessRequestV3) Request {
	return Request{
		ID:                 req.GetName(),
		User:               req.GetUser(),
		Roles:              req.GetRoles(),
		State:              req.GetState(),
		Created:            req.GetCreationTime(),
		RequestReason:      req.GetRequestReason(),
		ResolveReason:      req.GetResolveReason(),
		ResolveAnnotations: req.GetResolveAnnotations(),
		SystemAnnotations:  req.GetSystemAnnotations(),
		SuggestedReviewers: req.GetSuggestedReviewers(),
	}
}
