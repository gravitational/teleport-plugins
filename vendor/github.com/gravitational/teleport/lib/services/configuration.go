/*
Copyright 2017-2019 Gravitational, Inc.

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

package services

import "context"

// ClusterConfiguration stores the cluster configuration in the backend. All
// the resources modified by this interface can only have a single instance
// in the backend.
type ClusterConfiguration interface {
	// SetClusterName gets services.ClusterName from the backend.
	GetClusterName(opts ...MarshalOption) (ClusterName, error)
	// SetClusterName sets services.ClusterName on the backend.
	SetClusterName(ClusterName) error
	// UpsertClusterName upserts cluster name
	UpsertClusterName(ClusterName) error

	// DeleteClusterName deletes cluster name resource
	DeleteClusterName() error

	// GetStaticTokens gets services.StaticTokens from the backend.
	GetStaticTokens() (StaticTokens, error)
	// SetStaticTokens sets services.StaticTokens on the backend.
	SetStaticTokens(StaticTokens) error
	// DeleteStaticTokens deletes static tokens resource
	DeleteStaticTokens() error

	// GetAuthPreference gets services.AuthPreference from the backend.
	GetAuthPreference() (AuthPreference, error)
	// SetAuthPreference sets services.AuthPreference from the backend.
	SetAuthPreference(AuthPreference) error
	// DeleteAuthPreference deletes services.AuthPreference from the backend.
	DeleteAuthPreference(ctx context.Context) error

	// GetClusterConfig gets services.ClusterConfig from the backend.
	GetClusterConfig(opts ...MarshalOption) (ClusterConfig, error)
	// SetClusterConfig sets services.ClusterConfig on the backend.
	SetClusterConfig(ClusterConfig) error
	// DeleteClusterConfig deletes cluster config resource
	DeleteClusterConfig() error
}
