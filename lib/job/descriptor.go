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

type jobDescriptor struct {
	job     Job
	stopCtx context.Context
}

type jobDescriptorKey struct{}

func getJobDescriptor(ctx context.Context) (*jobDescriptor, bool) {
	desc, ok := ctx.Value(jobDescriptorKey{}).(*jobDescriptor)
	return desc, ok
}

func mustGetJobDescriptor(ctx context.Context) *jobDescriptor {
	if desc, ok := ctx.Value(jobDescriptorKey{}).(*jobDescriptor); ok {
		return desc
	}
	panic("running out of job context")
}
