/*
Copyright 2015-2021 Gravitational, Inc.

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

package test

import (
	"os/user"
	"testing"
	"time"

	"github.com/gravitational/teleport-plugins/lib"
	"github.com/gravitational/teleport-plugins/lib/logger"
	"github.com/gravitational/teleport-plugins/lib/testing/integration"
	"github.com/gravitational/teleport-plugins/terraform/provider"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/utils"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TerraformSuite struct {
	integration.AuthSetup
	// client represents plugin client
	client *integration.Client
	// teleportConfig represents Teleport configuration
	teleportConfig lib.TeleportConfig
	// teleportFeatures represents enabled Teleport feature flags
	teleportFeatures *proto.Features
	// plugin represents plugin user name
	plugin string
	// terraformConfig represents Terraform provider configuration
	terraformConfig string
	// terraformProviders represents an array of providers
	terraformProviders map[string]*schema.Provider
}

func TestTerraform(t *testing.T) { suite.Run(t, &TerraformSuite{}) }

func (s *TerraformSuite) SetupSuite() {
	var err error
	t := s.T()

	logger.Init()
	logger.Setup(logger.Config{Severity: "debug"})

	// We set such a big timeout because integration.NewFromEnv could start
	// downloading a Teleport *-bin.tar.gz file which can take a long time.
	ctx := s.SetContextTimeout(2 * time.Minute)

	s.AuthSetup.SetupSuite()
	s.AuthSetup.Setup()

	me, err := user.Current()
	require.NoError(t, err)

	client, err := s.Integration.MakeAdmin(ctx, s.Auth, me.Username+"-ruler@example.com")
	require.NoError(t, err)

	pong, err := client.Ping(ctx)
	require.NoError(t, err)
	teleportFeatures := pong.GetServerFeatures()

	var bootstrap integration.Bootstrap

	unrestricted := []string{"list", "create", "read", "update", "delete"}
	role, err := bootstrap.AddRole("terraform", types.RoleSpecV4{
		Allow: types.RoleConditions{
			DatabaseLabels: types.Labels(map[string]utils.Strings{"*": []string{"*"}}),
			AppLabels:      types.Labels(map[string]utils.Strings{"*": []string{"*"}}),
			Rules: []types.Rule{
				types.NewRule("token", unrestricted),
				types.NewRule("role", unrestricted),
				types.NewRule("user", unrestricted),
				types.NewRule("cluster_auth_preference", unrestricted),
				types.NewRule("cluster_networking_config", unrestricted),
				types.NewRule("session_recording_config", unrestricted),
				types.NewRule("db", unrestricted),
				types.NewRule("app", unrestricted),
				types.NewRule("github", unrestricted),
				types.NewRule("oidc", unrestricted),
				types.NewRule("saml", unrestricted),
			},
			Logins: []string{me.Username},
		},
	})
	require.NoError(t, err)

	user, err := bootstrap.AddUserWithRoles("terraform", role.GetName())
	require.NoError(t, err)
	s.plugin = user.GetName()

	err = s.Integration.Bootstrap(ctx, s.Auth, bootstrap.Resources())
	require.NoError(t, err)

	identityPath, err := s.Integration.Sign(ctx, s.Auth, s.plugin)
	require.NoError(t, err)

	s.client, err = s.Integration.NewClient(ctx, s.Auth, s.plugin)
	require.NoError(t, err)

	s.teleportConfig.Addr = s.Auth.AuthAddr().String()
	s.teleportConfig.Identity = identityPath
	s.teleportFeatures = teleportFeatures

	s.terraformConfig = `
		provider "teleport" {
			addr = "` + s.teleportConfig.Addr + `"
			identity_file_path = "` + s.teleportConfig.Identity + `"
		}
	`
	s.terraformProviders = map[string]*schema.Provider{"teleport": provider.Provider()}
}

func (s *TerraformSuite) SetupTest() {
}
