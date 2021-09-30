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

package job

import "context"

// SpawnOptions is job spawn options
type SpawnOptions struct {
	// Critical job, if returns error, leads to termination of the entire process.
	Critical bool
	// Readiness is a readiness status.
	Readiness *Readiness
	// ResultSetter is a setter called once the job is completed.
	ResultSetter ResultSetter

	stopped bool
}

// SpawnOption is a rest argument to Spawn and SpawnFunc methods of a process.
type SpawnOption func(*SpawnOptions)

// Critical marks a job as critical.
// Such job, if returns error, leads to termination of the entire process.
func Critical(critical bool) SpawnOption {
	return func(opts *SpawnOptions) {
		opts.Critical = critical
	}
}

// WithReadiness associates readiness object with a job.
func WithReadiness(readiness *Readiness) SpawnOption {
	return func(opts *SpawnOptions) {
		opts.Readiness = readiness
	}
}

// WithResult sets up a job result setter.
func WithResult(setter ResultSetter) SpawnOption {
	return func(opts *SpawnOptions) {
		opts.ResultSetter = setter
	}
}

// Spawn spawns a job in a process.
func (p *Process) Spawn(job Job, opts ...SpawnOption) {
	if p == nil {
		panic("spawning a job on a nil process")
	}
	var options SpawnOptions
	for _, optionFn := range opts {
		optionFn(&options)
	}
	p.spawn(job, options)
}

// SpawnFunc spawns a function as a job in a process.
func (p *Process) SpawnFunc(fn func(ctx context.Context) error, opts ...SpawnOption) {
	p.Spawn(FuncJob(fn), opts...)
}
