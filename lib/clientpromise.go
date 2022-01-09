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

package lib

import (
	"context"

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/trace"
)

// ClientPromise is a Teleport client being initizlied asynchronously.
type ClientPromise struct {
	doneCh <-chan struct{}
	clt    *client.Client
	err    error
}

// NewClientPromise builds a new ClientPromise given a factory function.
func NewClientPromise(connect func() (*client.Client, error)) *ClientPromise {
	doneCh := make(chan struct{})
	promise := ClientPromise{doneCh: doneCh}
	go func() {
		clt, err := connect()
		promise.clt, promise.err = clt, trace.Wrap(err)
		close(doneCh)
	}()
	return &promise
}

// GetClient waits for the client been initialized and returns the result.
func (promise *ClientPromise) GetClient(ctx context.Context) (*client.Client, error) {
	select {
	case <-promise.doneCh:
		return promise.clt, trace.Wrap(promise.err)
	case <-ctx.Done():
		return nil, trace.Wrap(ctx.Err())
	}
}
