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

package auth

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gravitational/trace"

	authv8 "github.com/gravitational/teleport-plugins/kubernetes/apis/auth/v8"
)

// SetupIndexes sets up an indexes of all the necessary fields of the resources in a client cache.
// Controller runtime requires indexes to be defined for any field you do filter on.
func SetupIndexes(ctx context.Context, cache cache.Cache) error {
	return trace.Wrap(
		cache.IndexField(ctx, &authv8.Identity{}, "spec.secretName", kclient.IndexerFunc(func(obj kclient.Object) []string {
			return []string{obj.(*authv8.Identity).Spec.SecretName}
		})),
	)
}
