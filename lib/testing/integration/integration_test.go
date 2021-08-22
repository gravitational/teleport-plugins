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
	"github.com/sirupsen/logrus"

	. "github.com/gravitational/teleport-plugins/lib/testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type IntegrationSuite struct {
	Suite
	integration *Integration
	auth        Service
	api         *API
}

func TestIntegration(t *testing.T) { suite.Run(t, &IntegrationSuite{}) }

func (s *IntegrationSuite) SetupTest() {
	t := s.T()
	var err error

	logrus.SetLevel(logrus.DebugLevel)

	s.SetContextTimeout(time.Second * 15)

	integration, err := NewFromEnv(s.Context())
	require.NoError(t, err)
	t.Cleanup(integration.Close)

	auth, err := integration.NewAuthServer()
	require.NoError(t, err)
	s.StartApp(auth)

	api, err := integration.NewAPI(s.Context(), auth)
	require.NoError(t, err)

	s.integration = integration
	s.auth = auth
	s.api = api
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

func (s *IntegrationSuite) TestCreateUser() {
	t := s.T()

	user, err := s.api.CreateUserWithRoles(s.Context(), "vladimir", "admin")
	require.NoError(t, err)
	assert.Equal(t, "vladimir", user.GetName())
	assert.Equal(t, []string{"admin"}, user.GetRoles())
}

func (s *IntegrationSuite) TestClient() {
	t := s.T()

	_, err := s.api.CreateUserWithRoles(s.Context(), "vladimir", "admin")
	require.NoError(t, err)

	client, err := s.integration.Client(s.Context(), s.auth, "vladimir")
	require.NoError(t, err)
	_, err = client.Ping(s.Context())
	require.NoError(t, err)
}
