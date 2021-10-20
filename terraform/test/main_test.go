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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TerraformSuite struct {
	integration.AuthSetup
	client            *integration.Client
	teleportConfig    lib.TeleportConfig
	teleportFeatures  *proto.Features
	plugin            string
	terraformConfig   string
	terraformProvider *schema.Provider
}

func TestTerraform(t *testing.T) { suite.Run(t, &TerraformSuite{}) }

func (s *TerraformSuite) SetupSuite() {
	var err error
	t := s.T()

	logger.Init()
	logger.Setup(logger.Config{Severity: "debug"})

	s.AuthSetup.SetupSuite()
	s.AuthSetup.Setup()

	// We set such a big timeout because integration.NewFromEnv could start
	// downloading a Teleport *-bin.tar.gz file which can take a long time.
	ctx := s.SetContextTimeout(2 * time.Minute)

	me, err := user.Current()
	require.NoError(t, err)

	client, err := s.Integration.MakeAdmin(ctx, s.Auth, me.Username+"-ruler@example.com")
	require.NoError(t, err)

	pong, err := client.Ping(ctx)
	require.NoError(t, err)
	teleportFeatures := pong.GetServerFeatures()

	var bootstrap integration.Bootstrap

	role, err := bootstrap.AddRole("terraform", types.RoleSpecV4{
		Allow: types.RoleConditions{
			Rules: []types.Rule{
				types.NewRule("provision_token", []string{"list", "create", "read", "update", "delete"}),
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
	s.terraformProvider = provider.Provider()
}

func (s *TerraformSuite) SetupTest() {
	s.SetContextTimeout(5 * time.Minute)
}

func (s *TerraformSuite) TestOk() {

}

// func testAccExampleResource(name string) string {
// 	return providerConfig + `
// 		resource "teleport_provision_token" "example" {
// 			metadata {
// 				name = "test2"
// 				expires = "2022-10-12T07:20:51.2Z"
// 				description = "Example token"

// 				labels = {
// 					example = "yes"
// 				}
// 			}

// 			spec {
// 				roles = ["Node", "Auth"]
// 			}
// 		}
// 	`
// }

// func testAccExampleResource_removedPolicy(name string) string {
// 	return providerConfig + `
// 	`
// }

// func testAccCheckExampleResourceExists(state *terraform.State) error {
// 	return nil
// }

// func TestAccExampleWidget_basic(t *testing.T) {
// 	//var widgetBefore, widgetAfter types.ProvisionTokenV2
// 	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

// 	resource.Test(t, resource.TestCase{
// 		//PreCheck:     func() { testAccPreCheck(t) },
// 		Providers: testAccProviders,
// 		//CheckDestroy: testAccCheckExampleResourceDestroy,
// 		Steps: []resource.TestStep{
// 			{
// 				Config: testAccExampleResource(rName),
// 				Check: resource.ComposeTestCheckFunc(
// 					testAccCheckExampleResourceExists,
// 				),
// 			},
// 			{
// 				Config: testAccExampleResource_removedPolicy(rName),
// 				Check: resource.ComposeTestCheckFunc(
// 					testAccCheckExampleResourceExists,
// 				),
// 			},
// 		},
// 	})
// }
