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
	"os/user"
	"testing"

	"github.com/gravitational/teleport-plugins/lib/tsh"
	"github.com/gravitational/teleport/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type IntegrationSSHSuite struct {
	SSHSetup
}

func TestIntegrationSSH(t *testing.T) { suite.Run(t, &IntegrationSSHSuite{}) }

func (s *IntegrationSSHSuite) SetupTest() {
	s.SSHSetup.SetupService()
}

func (s *IntegrationSSHSuite) TestBench() {
	t := s.T()
	me, err := user.Current()
	require.NoError(t, err)
	var bootstrap Bootstrap
	role, err := bootstrap.AddRole(me.Username, types.RoleSpecV4{Allow: types.RoleConditions{Logins: []string{me.Username}}})
	require.NoError(t, err)
	user, err := bootstrap.AddUserWithRoles(me.Username, role.GetName())
	require.NoError(t, err)
	err = s.Integration.Bootstrap(s.Context(), s.Auth, bootstrap.Resources())
	require.NoError(t, err)
	identityPath, err := s.Integration.Sign(s.Context(), s.Auth, user.GetName())
	require.NoError(t, err)
	tshCmd := s.Integration.NewTsh(s.Proxy.WebAndSSHProxyAddr(), identityPath)
	result, err := tshCmd.Bench(s.Context(), tsh.BenchFlags{}, user.GetName()+"@localhost", "ls")
	require.NoError(t, err)
	assert.Positive(t, result.RequestsOriginated)
	assert.Zero(t, result.RequestsFailed)
}
