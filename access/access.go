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
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/gravitational/trace"
	"github.com/hashicorp/go-version"

	"github.com/gravitational/teleport-plugins/utils"
	"github.com/gravitational/teleport/lib/auth/proto"
	"github.com/gravitational/teleport/lib/services"
)

const MinServerVersion = "4.2.2-alpha.1"

// State represents the state of an access request.
type State = services.RequestState

// StatePending is the state of a pending request.
const StatePending State = services.RequestState_PENDING

// StateApproved is the state of an approved request.
const StateApproved State = services.RequestState_APPROVED

// StateDenied is the state of a denied request.
const StateDenied State = services.RequestState_DENIED

// Op describes the operation type of an event.
type Op = proto.Operation

// OpInit is sent as the first sentinel value on the watch channel.
const OpInit = proto.Operation_INIT

// OpPut inicates creation or update.
const OpPut = proto.Operation_PUT

// OpDelete indicates deletion or expiry.
const OpDelete = proto.Operation_DELETE

// Filter encodes request filtering parameters.
type Filter = services.AccessRequestFilter

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
}

// Pong describes a ping response.
type Pong struct {
	ServerVersion string
	ClusterName   string
}

// PluginData is a custom user data associated with access request.
type PluginData map[string]string

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
	// Ping loads basic information about Teleport version and cluster name
	Ping(ctx context.Context) (Pong, error)
	// WatchRequests registers a new watcher for pending access requests.
	WatchRequests(ctx context.Context, fltr Filter) Watcher
	// GetRequests loads all requests which match provided filter.
	GetRequests(ctx context.Context, fltr Filter) ([]Request, error)
	// GetRequest loads a request matching ID.
	GetRequest(ctx context.Context, reqID string) (Request, error)
	// SetRequestState updates the state of a request.
	SetRequestState(ctx context.Context, reqID string, state State) error
	// GetPluginData fetches plugin data of the specific request.
	GetPluginData(ctx context.Context, reqID string) (PluginData, error)
	// UpdatePluginData updates plugin data of the specific request comparing it with a previous value.
	UpdatePluginData(ctx context.Context, reqID string, set PluginData, expect PluginData) error
}

// clt is a thin wrapper around the raw GRPC types that implements the
// access.Client interface.
type clt struct {
	plugin string
	clt    proto.AuthServiceClient
	cancel context.CancelFunc
}

func NewClient(ctx context.Context, plugin string, addr string, tc *tls.Config) (Client, error) {
	ctx, cancel := context.WithCancel(ctx)
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(credentials.NewTLS(tc)),
		grpc.WithBackoffMaxDelay(time.Second*2),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	)
	if err != nil {
		cancel()
		return nil, utils.FromGRPC(err)
	}
	authClient := proto.NewAuthServiceClient(conn)
	return &clt{
		plugin: plugin,
		clt:    authClient,
		cancel: cancel,
	}, nil
}

func (c *clt) Ping(ctx context.Context) (Pong, error) {
	rsp, err := c.clt.Ping(ctx, &proto.PingRequest{})
	if err != nil {
		return Pong{}, utils.FromGRPC(err)
	}
	return Pong{
		rsp.ServerVersion,
		rsp.ClusterName,
	}, nil
}

func (c *clt) WatchRequests(ctx context.Context, fltr Filter) Watcher {
	return newWatcher(ctx, c.clt, fltr)
}

func (c *clt) GetRequests(ctx context.Context, fltr Filter) ([]Request, error) {
	rsp, err := c.clt.GetAccessRequests(ctx, &fltr)
	if err != nil {
		return nil, utils.FromGRPC(err)
	}
	var reqs []Request
	for _, req := range rsp.AccessRequests {
		r := Request{
			ID:      req.GetName(),
			User:    req.GetUser(),
			Roles:   req.GetRoles(),
			State:   req.GetState(),
			Created: req.GetCreationTime(),
		}
		reqs = append(reqs, r)
	}
	return reqs, nil
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

func (c *clt) SetRequestState(ctx context.Context, reqID string, state State) error {
	_, err := c.clt.SetAccessRequestState(ctx, &proto.RequestStateSetter{
		ID:    reqID,
		State: state,
	})
	return utils.FromGRPC(err)
}

func (c *clt) GetPluginData(ctx context.Context, reqID string) (PluginData, error) {
	dataSeq, err := c.clt.GetPluginData(ctx, &services.PluginDataFilter{
		Kind:     services.KindAccessRequest,
		Resource: reqID,
		Plugin:   c.plugin,
	})
	if err != nil {
		return nil, utils.FromGRPC(err)
	}
	pluginDatas := dataSeq.GetPluginData()
	if len(pluginDatas) == 0 {
		return PluginData{}, nil
	}

	var pluginData services.PluginData = pluginDatas[0]
	entry := pluginData.Entries()[c.plugin]
	if entry == nil {
		return PluginData{}, nil
	}
	return entry.Data, nil
}

func (c *clt) UpdatePluginData(ctx context.Context, reqID string, set PluginData, expect PluginData) (err error) {
	_, err = c.clt.UpdatePluginData(ctx, &services.PluginDataUpdateParams{
		Kind:     services.KindAccessRequest,
		Resource: reqID,
		Plugin:   c.plugin,
		Set:      set,
		Expect:   expect,
	})
	return utils.FromGRPC(err)
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

func newWatcher(ctx context.Context, clt proto.AuthServiceClient, fltr Filter) *watcher {
	ctx, cancel := context.WithCancel(ctx)
	w := &watcher{
		eventC: make(chan Event),
		initC:  make(chan struct{}),
		doneC:  make(chan struct{}),
		cancel: cancel,
	}
	go w.run(ctx, clt, fltr)
	return w
}

func (w *watcher) run(ctx context.Context, clt proto.AuthServiceClient, fltr Filter) {
	defer w.Close()
	defer close(w.doneC)

	stream, err := clt.WatchEvents(ctx, &proto.Watch{
		Kinds: []proto.WatchKind{
			proto.WatchKind{
				Kind:   services.KindAccessRequest,
				Filter: fltr.IntoMap(),
			},
		},
	})
	if err != nil {
		w.setError(utils.FromGRPC(err))
		return
	}

	for {
		event, err := stream.Recv()
		if err != nil {
			w.setError(utils.FromGRPC(err))
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
			req = Request{
				ID:      r.GetName(),
				User:    r.GetUser(),
				Roles:   r.GetRoles(),
				State:   r.GetState(),
				Created: r.GetCreationTime(),
			}
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
func (p *Pong) AssertServerVersion() error {
	actual, err := version.NewVersion(p.ServerVersion)
	if err != nil {
		return trace.Wrap(err)
	}
	required, err := version.NewVersion(MinServerVersion)
	if err != nil {
		return trace.Wrap(err)
	}
	if actual.LessThan(required) {
		return trace.Errorf("server version %s is less than %s", p.ServerVersion, MinServerVersion)
	}
	return nil
}
