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

	"github.com/gravitational/teleport-plugins/lib/tctl"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"google.golang.org/grpc"
)

const (
	DefaultLocalAddr  = "127.0.0.1:3025"
	DefaultConfigPath = "/etc/teleport.yaml"
)

// SidecarOptions configure the sidecar connection.
type SidecarOptions struct {
	// ConfigPath is a path to the Teleport configuration file e.g. /etc/teleport.yaml.
	ConfigPath string

	// Addr is an endpoint of Teleport e.g. 127.0.0.1:3025.
	Addr string

	// User is a user used to access Teleport Auth/Proxy/Tunnel server.
	User string

	// Role is a role allowed to manage Teleport resources.
	Role string

	// DialOpts define options for dialing the client connection.
	DialOpts []grpc.DialOption

	// MinServerVersion is a minimum version.
	MinServerVersion string
}

// NewSidecarClient returns a connection to the Teleport server running on the same machine or pod.
// It automatically upserts the sidecar role and the user and generates the credentials.
func NewSidecarClient(ctx context.Context, opts SidecarOptions) (*client.Client, error) {
	role, err := types.NewRole(opts.Role, types.RoleSpecV4{
		Allow: types.RoleConditions{
			Rules: []types.Rule{
				types.Rule{
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
		},
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	user, err := types.NewUser(opts.User)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	user.AddRole(role.GetName())

	tctl := tctl.Tctl{ConfigPath: opts.ConfigPath, AuthServer: opts.Addr}
	if err = tctl.Create(ctx, []types.Resource{role, user}, true); err != nil {
		return nil, trace.Wrap(err)
	}

	identity, err := tctl.SignToString(ctx, user.GetName(), 0)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return client.New(ctx, client.Config{
		Addrs:       []string{opts.Addr},
		Credentials: []client.Credentials{client.LoadIdentityFileFromString(identity)},
		DialOpts:    opts.DialOpts,
	})
}

func (opts *SidecarOptions) CheckAndSetDefaults() error {
	if opts.Addr == "" {
		opts.Addr = DefaultLocalAddr
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = DefaultConfigPath
	}
	if opts.User == "" {
		return trace.BadParameter("sidecar user is not set")
	}
	if opts.Role == "" {
		return trace.BadParameter("sidecar role is not set")
	}
	return nil
}
