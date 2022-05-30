/*
Copyright 2022 Gravitational, Inc.

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

package sidecar

import (
	"context"
	"os"

	"github.com/gravitational/teleport-plugins/lib/tctl"
	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"google.golang.org/grpc"
)

const (
	DefaultLocalAddr  = "127.0.0.1:3025"
	DefaultConfigPath = "/etc/teleport/teleport.yaml"
	DefaultUser       = "teleport-operator-sidecar"
	DefaultRole       = "teleport-operator-sidecar"
)

// Options configure the sidecar connection.
type Options struct {
	// ConfigPath is a path to the Teleport configuration file e.g. /etc/teleport/teleport.yaml.
	ConfigPath string

	// Addr is an endpoint of Teleport e.g. 127.0.0.1:3025.
	Addr string

	// User is a user used to access Teleport Auth/Proxy/Tunnel server.
	User string

	// Role is a role allowed to manage Teleport resources.
	Role string

	// DialOpts define options for dialing the client connection.
	DialOpts []grpc.DialOption
}

// NewSidecarClient returns a connection to the Teleport server running on the same machine or pod.
// It automatically upserts the sidecar role and the user and generates the credentials.
func NewSidecarClient(ctx context.Context, opts Options) (*client.Client, error) {
	if err := opts.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	tctl := tctl.Tctl{ConfigPath: opts.ConfigPath, AuthServer: opts.Addr}

	resourcesToCreate := make([]types.Resource, 0)

	exists, err := tctl.Exists(ctx, types.KindRole, opts.Role)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query role")
	}
	if !exists {
		role, err := sidecarRole(opts.Role)
		if err != nil {
			return nil, trace.Wrap(err, "failed to create role")
		}
		resourcesToCreate = append(resourcesToCreate, role)
	}

	exists, err = tctl.Exists(ctx, types.KindUser, opts.User)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query user")
	}
	if !exists {
		user, err := sidecarUserWithRole(opts.User, opts.Role)
		if err != nil {
			return nil, trace.Wrap(err, "failed to create user")
		}
		resourcesToCreate = append(resourcesToCreate, user)
	}

	if len(resourcesToCreate) > 0 {
		if err := tctl.Create(ctx, resourcesToCreate); err != nil {
			return nil, trace.Wrap(err, "failed to create resources in Teleport")
		}
	}

	identityfile, err := os.CreateTemp("", "teleport-identity-*")
	if err != nil {
		return nil, trace.Wrap(err, "failed to create temp identity file")
	}
	defer os.Remove(identityfile.Name())

	if err := tctl.Sign(ctx, opts.User, "file", identityfile.Name()); err != nil {
		return nil, trace.Wrap(err, "failed to write identity file")
	}

	return client.New(ctx, client.Config{
		Addrs:       []string{opts.Addr},
		Credentials: []client.Credentials{client.LoadIdentityFile(identityfile.Name())},
		DialOpts:    opts.DialOpts,
	})
}

func sidecarRole(roleName string) (types.Role, error) {
	return types.NewRoleV5(roleName, types.RoleSpecV5{
		Allow: types.RoleConditions{
			Rules: []types.Rule{
				{
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
		},
	})
}

func sidecarUserWithRole(userName, roleName string) (types.User, error) {
	user, err := types.NewUser(userName)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	user.AddRole(roleName)

	return user, nil
}

func (opts *Options) CheckAndSetDefaults() error {
	if opts.Addr == "" {
		opts.Addr = DefaultLocalAddr
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = DefaultConfigPath
	}
	if opts.User == "" {
		opts.User = DefaultUser
	}
	if opts.Role == "" {
		opts.Role = DefaultRole
	}
	return nil
}
