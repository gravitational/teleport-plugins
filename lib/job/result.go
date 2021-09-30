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

import "sync"

// FutureResult describes a result of job computation.
type FutureResult interface {
	Future
	ResultSetter
}

// Future serves for synchronization of concurrent computation.
type Future interface {
	// Done is a completion channel of the future.
	Done() <-chan struct{}
	// Err is a future result.
	Err() error
}

// ResultSetter is a setter of computation result.
type ResultSetter interface {
	SetError(error)
}

func NewFutureResult() FutureResult {
	return &futureResult{doneCh: make(chan struct{})}
}

type futureResult struct {
	mu     sync.Mutex
	doneCh chan struct{}
	err    error
}

func (result *futureResult) Done() <-chan struct{} {
	return result.doneCh
}

func (result *futureResult) Err() error {
	result.mu.Lock()
	defer result.mu.Unlock()
	return result.err
}

func (result *futureResult) SetError(err error) {
	result.mu.Lock()
	defer result.mu.Unlock()
	select {
	case <-result.doneCh:
		// result already set
	default:
		result.err = err
		close(result.doneCh)
	}
}
