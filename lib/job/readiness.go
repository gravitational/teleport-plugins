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

package job

import (
	"context"
	"sync"

	"github.com/gravitational/trace"
)

type Readiness struct {
	mu     sync.Mutex
	ready  bool
	doneCh chan struct{}
}

type readinessKey struct{}

var alreadyDone = make(chan struct{})

func init() {
	close(alreadyDone)
}

// SetReady sets a job readiness status.
func SetReady(ctx context.Context, ready bool) {
	if readiness, ok := ctx.Value(readinessKey{}).(*Readiness); ok {
		readiness.setReady(ready)
	}
}

// IsReady returns a readiness status.
func (readiness *Readiness) IsReady() bool {
	readiness.mu.Lock()
	defer readiness.mu.Unlock()
	return readiness.ready
}

// WaitReady waits for readiness status to be set or ctx is done.
func (readiness *Readiness) WaitReady(ctx context.Context) (bool, error) {
	select {
	case <-readiness.Done():
		return readiness.IsReady(), nil
	case <-ctx.Done():
		return false, trace.Wrap(ctx.Err())
	}
}

// Done returns a channel which is closed when readiness status is set.
func (readiness *Readiness) Done() <-chan struct{} {
	readiness.mu.Lock()
	defer readiness.mu.Unlock()
	if readiness.doneCh == nil {
		readiness.doneCh = make(chan struct{})
	}
	return readiness.doneCh
}

func (readiness *Readiness) setReady(ready bool) {
	readiness.mu.Lock()
	defer readiness.mu.Unlock()

	readiness.ready = ready
	select {
	case <-readiness.doneCh:
	default:
		if readiness.doneCh != nil {
			close(readiness.doneCh)
		} else {
			readiness.doneCh = alreadyDone
		}
	}
}
