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
	"io"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/gravitational/trace"
	"github.com/gravitational/trace/trail"
	"github.com/hashicorp/go-version"

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
}

// Pong describes a ping response.
type Pong struct {
	ServerVersion string
	ClusterName   string
}

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
	// SetRequestState updates the state of a request.
	SetRequestState(ctx context.Context, reqID string, state State) error
}

// clt is a thin wrapper around the raw GRPC types that implements the
// access.Client interface.
type clt struct {
	clt    proto.AuthServiceClient
	cancel context.CancelFunc
}

func NewClient(ctx context.Context, addr string, tc *tls.Config) (Client, error) {
	ctx, cancel := context.WithCancel(ctx)
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(credentials.NewTLS(tc)),
		grpc.WithBackoffMaxDelay(time.Second*2),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	)
	if err != nil {
		cancel()
		return nil, fromGRPC(err)
	}
	return &clt{
		clt:    proto.NewAuthServiceClient(conn),
		cancel: cancel,
	}, nil
}

func (c *clt) Ping(ctx context.Context) (Pong, error) {
	rsp, err := c.clt.Ping(ctx, &proto.PingRequest{})
	if err != nil {
		return Pong{}, fromGRPC(err)
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
		return nil, fromGRPC(err)
	}
	var reqs []Request
	for _, req := range rsp.AccessRequests {
		r := Request{
			ID:    req.GetName(),
			User:  req.GetUser(),
			Roles: req.GetRoles(),
			State: req.GetState(),
		}
		reqs = append(reqs, r)
	}
	return reqs, nil
}

func (c *clt) SetRequestState(ctx context.Context, reqID string, state State) error {
	// this function attempts a small number of retries if updating request state
	// fails due to concurrent updates.  Since the auth server ensures that a
	// denied request cannot be subsequently approved, retries are safe.
	const maxAttempts = 3
	const baseSleep = time.Millisecond * 199
	var err error
	for i := 1; i <= maxAttempts; i++ {
		err = c.setRequestState(ctx, reqID, state)
		if !trace.IsCompareFailed(err) {
			return trace.Wrap(err)
		}
		select {
		case <-time.After(baseSleep * time.Duration(i)):
		case <-ctx.Done():
			return trace.Wrap(err)
		}
	}
	return trace.Wrap(err)
}

func (c *clt) setRequestState(ctx context.Context, reqID string, state State) error {
	_, err := c.clt.SetAccessRequestState(ctx, &proto.RequestStateSetter{
		ID:    reqID,
		State: state,
	})
	return fromGRPC(err)
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

// TODO: remove this when trail.FromGRPC will understand additional error codes
func fromGRPC(err error) error {
	switch {
	case err == io.EOF:
		fallthrough
	case status.Code(err) == codes.Canceled, err == context.Canceled:
		fallthrough
	case status.Code(err) == codes.DeadlineExceeded, err == context.DeadlineExceeded:
		return trace.Wrap(err)
	default:
		return trail.FromGRPC(err)
	}
}

// TODO: remove this when trail.FromGRPC will understand additional error codes
func IsCanceled(err error) bool {
	err = trace.Unwrap(err)
	return err == context.Canceled || status.Code(err) == codes.Canceled
}

// TODO: remove this when trail.FromGRPC will understand additional error codes
func IsDeadline(err error) bool {
	err = trace.Unwrap(err)
	return err == context.DeadlineExceeded || status.Code(err) == codes.DeadlineExceeded
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
		w.setError(fromGRPC(err))
		return
	}

	for {
		event, err := stream.Recv()
		if err != nil {
			w.setError(fromGRPC(err))
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
				ID:    r.GetName(),
				User:  r.GetUser(),
				Roles: r.GetRoles(),
				State: r.GetState(),
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
