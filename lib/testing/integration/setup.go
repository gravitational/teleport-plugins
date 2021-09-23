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
	"time"

	"github.com/gravitational/teleport-plugins/lib/logger"
	. "github.com/gravitational/teleport-plugins/lib/testing"

	"github.com/stretchr/testify/require"
)

type IntegrationSetup struct {
	Suite
	Integration *Integration
}

type IntegrationAuthSetup struct {
	IntegrationSetup
	Auth *AuthService
}

type IntegrationProxySetup struct {
	IntegrationAuthSetup
	Proxy *ProxyService
}

type IntegrationSSHSetup struct {
	IntegrationProxySetup
	SSH *SSHService
}

func (s *IntegrationSetup) SetupSuite() {
	logger.Init()
	logger.Setup(logger.Config{Severity: "debug"})
}

func (s *IntegrationSetup) Setup() {
	t := s.T()
	var err error

	// We set such a big timeout because integration.NewFromEnv could start
	// downloading a Teleport *-bin.tar.gz file which can take a long time.
	ctx := s.SetContextTimeout(2 * time.Minute)
	integration, err := NewFromEnv(ctx)
	require.NoError(t, err)
	t.Cleanup(integration.Close)
	s.Integration = integration

	s.SetContextTimeout(time.Second * 15)
}

func (s *IntegrationAuthSetup) SetupSuite() {
	s.IntegrationSetup.SetupSuite()
}

func (s *IntegrationAuthSetup) Setup() {
	s.IntegrationSetup.Setup()
	t := s.T()
	auth, err := s.Integration.NewAuthService()
	require.NoError(t, err)
	s.StartApp(auth)
	s.Auth = auth

	// Set CA Pin so that Proxy and SSH can register to auth securely.
	err = s.Integration.SetCAPin(s.Context(), s.Auth)
	require.NoError(t, err)
}

func (s *IntegrationProxySetup) SetupSuite() {
	s.IntegrationAuthSetup.SetupSuite()
}

func (s *IntegrationProxySetup) Setup() {
	s.IntegrationAuthSetup.Setup()
	t := s.T()
	proxy, err := s.Integration.NewProxyService(s.Auth)
	require.NoError(t, err)
	s.StartApp(proxy)
	s.Proxy = proxy
}

func (s *IntegrationSSHSetup) SetupSuite() {
	s.IntegrationProxySetup.SetupSuite()
}

func (s *IntegrationSSHSetup) SetupTest() {
	s.IntegrationProxySetup.Setup()
	t := s.T()
	ssh, err := s.Integration.NewSSHService(s.Auth)
	require.NoError(t, err)
	s.StartApp(ssh)
	s.SSH = ssh
}
