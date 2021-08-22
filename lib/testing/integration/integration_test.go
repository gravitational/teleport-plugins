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
	"testing"
	"time"

	"github.com/hashicorp/go-version"

	"github.com/gravitational/teleport-plugins/lib/logger"
	. "github.com/gravitational/teleport-plugins/lib/testing"
	"github.com/gravitational/teleport/api/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type IntegrationSuite struct {
	Suite
	integration *Integration
}

type IntegrationAuthSuite struct {
	IntegrationSuite
	auth *AuthServer
}

func TestIntegration(t *testing.T) { suite.Run(t, &IntegrationSuite{}) }

func (s *IntegrationSuite) SetupSuite() {
	logger.Init()
	logger.Setup(logger.Config{Severity: "debug"})
}

func (s *IntegrationSuite) SetupTest() {
	t := s.T()
	var err error

	integration, err := NewFromEnv(s.Context())
	require.NoError(t, err)
	t.Cleanup(integration.Close)
	s.integration = integration

	s.SetContextTimeout(time.Second * 15)
}

func (s *IntegrationSuite) TestVersion() {
	t := s.T()

	versionMin, err := version.NewVersion("v6.2.7")
	require.NoError(t, err)
	versionMax, err := version.NewVersion("v8")
	require.NoError(t, err)

	assert.True(t, s.integration.Version().GreaterThanOrEqual(versionMin))
	assert.True(t, s.integration.Version().LessThan(versionMax))
}

func TestIntegrationAuth(t *testing.T) { suite.Run(t, &IntegrationAuthSuite{}) }

func (s *IntegrationAuthSuite) SetupSuite() {
	s.IntegrationSuite.SetupSuite()
}

func (s *IntegrationAuthSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	t := s.T()
	auth, err := s.integration.NewAuthServer()
	require.NoError(t, err)
	s.StartApp(auth)
	s.auth = auth
}

func (s *IntegrationAuthSuite) TestBootstrap() {
	t := s.T()

	var bootstrap Bootstrap
	role, err := bootstrap.AddRole("foo", types.RoleSpecV4{})
	require.NoError(t, err)
	_, err = bootstrap.AddUserWithRoles("vladimir", "admin", role.GetName())
	require.NoError(t, err)
	err = s.integration.Bootstrap(s.Context(), s.auth, bootstrap.Resources())
	require.NoError(t, err)
}

func (s *IntegrationAuthSuite) TestClient() {
	t := s.T()

	var bootstrap Bootstrap
	user, err := bootstrap.AddUserWithRoles("vladimir", "admin")
	require.NoError(t, err)
	err = s.integration.Bootstrap(s.Context(), s.auth, bootstrap.Resources())
	require.NoError(t, err)

	client, err := s.integration.NewClient(s.Context(), s.auth, user.GetName())
	require.NoError(t, err)
	_, err = client.Ping(s.Context())
	require.NoError(t, err)
}
