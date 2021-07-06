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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func measure(fn func()) time.Duration {
	before := time.Now()
	fn()
	after := time.Now()
	return after.Sub(before)
}

func TestDecorr(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	base := 200 * time.Millisecond
	cap := 2 * time.Second
	delay := 125 * time.Millisecond // Lets have some delay because darwin builds on CI are a bit laggy.
	backoff := Decorr(base, cap)

	// Check exponential bounds.
	for max := 3 * base; max < cap; max = 3 * max {
		dur := measure(func() { require.NoError(t, backoff.Do(ctx)) })
		require.Greater(t, dur, base)
		require.Less(t, dur, max+delay)
	}

	// Check that exponential growth threshold.
	for i := 0; i < 2; i++ {
		dur := measure(func() { require.NoError(t, backoff.Do(ctx)) })
		require.Greater(t, dur, base)
		require.Less(t, dur, cap+delay)
	}
}
