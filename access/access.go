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

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/gravitational/trace"
	"github.com/gravitational/trace/trail"

	"github.com/gravitational/teleport/lib/auth/proto"
	"github.com/gravitational/teleport/lib/services"
)

// State represents the state of an access request.
type State = services.RequestState

// Request describes a pending access request.
type Request struct {
	ID    string
	User  string
	Roles []string
	State State
}

// Watcher is used to stream pending access requests as they are
// created by users.
type Watcher interface {
	Requests() <-chan Request
	Done() <-chan struct{}
	Error() error
	Close()
}

// Client is an access request management client.
type Client interface {
	// WatchRequests registers a new watcher for pending access requests.
	WatchRequests(ctx context.Context) (Watcher, error)
	// GetRequest loads an access request.
	GetRequest(ctx context.Context, reqID string) (Request, error)
	ApproveRequest(ctx context.Context, reqID string) error
	DenyRequest(ctx context.Context, reqID string) error
}

// clt is a thin wrapper around the raw GRPC types that implements the
// access.Client interface.
type clt struct {
	clt    proto.AuthServiceClient
	cancel context.CancelFunc
}

func NewClient(ctx context.Context, addr string, tc *tls.Config) (Client, error) {
	ctx, cancel := context.WithCancel(ctx)
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(credentials.NewTLS(tc)))
	if err != nil {
		cancel()
		return nil, trail.FromGRPC(err)
	}
	return &clt{
		clt:    proto.NewAuthServiceClient(conn),
		cancel: cancel,
	}, nil
}

func (c *clt) WatchRequests(ctx context.Context) (Watcher, error) {
	watcher, err := newWatcher(ctx, c.clt)
	return watcher, trace.Wrap(err)
}

func (c *clt) GetRequest(ctx context.Context, reqID string) (Request, error) {
	rsp, err := c.clt.GetAccessRequests(ctx, &services.AccessRequestFilter{
		ID: reqID,
	})
	if err != nil {
		return Request{}, trail.FromGRPC(err)
	}
	if len(rsp.AccessRequests) < 1 {
		return Request{}, trace.NotFound("no request matching %q", reqID)
	}
	req := rsp.AccessRequests[0]
	return Request{
		ID:    req.GetName(),
		User:  req.GetUser(),
		Roles: req.GetRoles(),
		State: req.GetState(),
	}, nil
}

func (c *clt) ApproveRequest(ctx context.Context, reqID string) error {
	_, err := c.clt.SetAccessRequestState(ctx, &proto.RequestStateSetter{
		ID:    reqID,
		State: services.RequestState_APPROVED,
	})
	return trail.FromGRPC(err)
}

func (c *clt) DenyRequest(ctx context.Context, reqID string) error {
	_, err := c.clt.SetAccessRequestState(ctx, &proto.RequestStateSetter{
		ID:    reqID,
		State: services.RequestState_DENIED,
	})
	return trail.FromGRPC(err)
}

func (c *clt) Close() {
	c.cancel()
}

type watcher struct {
	stream proto.AuthService_WatchAccessRequestsClient
	reqC   chan Request
	ctx    context.Context
	emux   sync.Mutex
	err    error
	cancel context.CancelFunc
}

func newWatcher(ctx context.Context, clt proto.AuthServiceClient) (*watcher, error) {
	ctx, cancel := context.WithCancel(ctx)
	stream, err := clt.WatchAccessRequests(ctx, &services.AccessRequestFilter{
		State: services.RequestState_PENDING,
	})
	if err != nil {
		cancel()
		return nil, trail.FromGRPC(err)
	}
	w := &watcher{
		stream: stream,
		reqC:   make(chan Request),
		ctx:    ctx,
		cancel: cancel,
	}
	go w.run()
	return w, nil
}

func (w *watcher) run() {
	defer w.cancel()
	for {
		req, err := w.stream.Recv()
		if err != nil {
			w.setError(trail.FromGRPC(err))
			return
		}
		w.reqC <- Request{
			ID:    req.GetName(),
			User:  req.GetUser(),
			Roles: req.GetRoles(),
			State: req.GetState(),
		}
	}
}

func (w *watcher) Requests() <-chan Request {
	return w.reqC
}

func (w *watcher) Done() <-chan struct{} {
	return w.ctx.Done()
}

func (w *watcher) Error() error {
	w.emux.Lock()
	defer w.emux.Unlock()
	if w.err != nil {
		return w.err
	}
	return w.ctx.Err()
}

func (w *watcher) setError(err error) {
	w.emux.Lock()
	defer w.emux.Unlock()
	w.err = err
}

func (w *watcher) Close() {
	w.cancel()
}
