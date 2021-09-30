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

type Process struct {
	// doneCh is closed when all the jobs are completed.
	doneCh <-chan struct{}
	// spawn runs a goroutine in the app's context as a job with waiting for
	// its completion on shutdown.
	spawn func(Job, SpawnOptions)
	// stop signals the app to terminate gracefully.
	stop func()
	// cancel signals the app to terminate immediately
	cancel context.CancelFunc
}

type jobGroup struct {
	mu      sync.Mutex
	counter uint
	doneCh  chan struct{}
}

type processKey struct{}

func NewProcess(ctx context.Context) *Process {
	// onStop is a list of callbacks called on terminate.
	var onStop sync.Map

	group := newJobGroup()
	ctx, cancel := context.WithCancel(ctx)
	process := &Process{
		doneCh: group.done(),
		cancel: cancel,
	}
	ctx = context.WithValue(ctx, processKey{}, process)

	process.spawn = func(job Job, opts SpawnOptions) {
		group.join()

		desc := &jobDescriptor{job: job}
		jobCtx, jcancel := context.WithCancel(ctx)
		if opts.Readiness != nil {
			jobCtx = context.WithValue(jobCtx, readinessKey{}, opts.Readiness)
		}
		jobCtx = context.WithValue(jobCtx, jobDescriptorKey{}, desc)
		stopCtx, stop := context.WithCancel(jobCtx)
		desc.stopCtx = stopCtx
		if !opts.stopped {
			onStop.Store(desc, FuncJob(func(context.Context) error {
				stop()
				return nil
			}))
		} else {
			stop()
		}
		result := opts.ResultSetter

		go func() {
			defer func() {
				jcancel()
				onStop.Delete(desc)
				group.leave()
			}()
			err := trace.Wrap(job.DoJob(jobCtx))
			if result != nil {
				result.SetError(err)
			}
			if err != nil && opts.Critical {
				process.Stop()
			}
		}()
	}

	var stopOnce sync.Once
	process.stop = func() {
		stopOnce.Do(func() {
			onStop.Range(func(desc, job interface{}) bool {
				onStop.Delete(desc)
				process.spawn(job.(FuncJob), SpawnOptions{stopped: true})
				return true
			})
			group.leave() // Stop the main "job".
		})
	}

	return process
}

// Done channel is used to wait for jobs completion.
func (p *Process) Done() <-chan struct{} {
	if p == nil {
		return alreadyDone
	}
	return p.doneCh
}

// Stop signals a process to terminate. You should avoid spawning new jobs after stopping.
func (p *Process) Stop() {
	if p == nil {
		return
	}
	p.stop()
}

// Shutdown signals a process to terminate and waits for completion of all jobs.
func (p *Process) Shutdown(ctx context.Context) error {
	p.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.Done():
		return nil
	}
}

// Close shuts down all process jobs immediately.
func (p *Process) Close() {
	if p == nil {
		return
	}
	p.cancel()
	<-p.doneCh
}

// GetProcess gets a currently running job's process.
func GetProcess(ctx context.Context) *Process {
	if process, ok := ctx.Value(processKey{}).(*Process); ok {
		return process
	}
	return nil
}

// MustGetProcess gets a currently running job's process or panics if it's out of job context.
func MustGetProcess(ctx context.Context) *Process {
	if process, ok := ctx.Value(processKey{}).(*Process); ok {
		return process
	}
	panic("running out of process context")
}

func newJobGroup() *jobGroup {
	return &jobGroup{
		doneCh:  make(chan struct{}),
		counter: 1, // ONE means a single main "job".
	}
}

func (jobs *jobGroup) join() {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if jobs.counter == 0 {
		panic("failed to spawn job: process already finished")
	}
	jobs.counter++
}

func (jobs *jobGroup) leave() {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if jobs.counter == 0 {
		panic("failed to decrement zero job counter")
	}
	jobs.counter--
	if jobs.counter == 0 {
		close(jobs.doneCh)
	}
}

func (jobs *jobGroup) done() <-chan struct{} {
	return jobs.doneCh
}
