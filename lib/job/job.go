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

// Job is just something executable.
type Job interface {
	// DoJob executes a job.
	DoJob(context.Context) error
}

// FuncJob is a simplest job represented as a mere function.
type FuncJob func(context.Context) error

// GetJob gets a currently running job.
func GetJob(ctx context.Context) Job {
	if desc, ok := getJobDescriptor(ctx); ok {
		return desc.job
	}
	return nil
}

// MustGetJob gets a currently running job or panics if it's out of job context.
func MustGetJob(ctx context.Context) Job {
	return mustGetJobDescriptor(ctx).job
}

// DoJob executes a job.
func (j FuncJob) DoJob(ctx context.Context) error {
	return j(ctx)
}

// Stopped returns a channel closed once a job / entire process is signaled to stop.
func Stopped(ctx context.Context) <-chan struct{} {
	if desc, ok := ctx.Value(jobDescriptorKey{}).(*jobDescriptor); ok {
		return desc.stopCtx.Done()
	}
	return nil
}
