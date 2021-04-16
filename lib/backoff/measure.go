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

package backoff

import (
	"context"
	"runtime"
	"time"

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
)

func measure(ctx context.Context, clock clockwork.FakeClock, fn func() error) (time.Duration, error) {
	done := make(chan struct{})
	var dur time.Duration
	var err error
	go func() {
		before := clock.Now()
		err = fn()
		after := clock.Now()
		dur = after.Sub(before)
		close(done)
	}()
	clock.BlockUntil(1)
	for {
		clock.Advance(5 * time.Millisecond)
		runtime.Gosched() // Nothing works without it :(
		select {
		case <-done:
			return dur, trace.Wrap(err)
		case <-ctx.Done():
			return time.Duration(0), trace.Wrap(ctx.Err())
		default:
		}
	}
}
