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

package integration

import (
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

type Bootstrap struct {
	resources []types.Resource
}

func (bootstrap *Bootstrap) Add(resource types.Resource) {
	bootstrap.resources = append(bootstrap.resources, resource)
}

func (bootstrap *Bootstrap) Resources() []types.Resource {
	return bootstrap.resources
}

func (bootstrap *Bootstrap) AddUserWithRoles(name string, roles ...string) (types.User, error) {
	user, err := types.NewUser(name)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	user.SetRoles(roles)
	bootstrap.Add(user)
	return user, nil
}

func (bootstrap *Bootstrap) AddRole(name string, spec types.RoleSpecV5) (types.Role, error) {
	role, err := types.NewRoleV3(name, spec)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	bootstrap.Add(role)
	return role, nil
}
