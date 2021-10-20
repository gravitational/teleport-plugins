package test

import (
	"testing"

	"github.com/gravitational/teleport-plugins/terraform/provider"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// type EventHandlerSuite struct {
// 	integration.SSHSetup
// 	clients        map[string]*integration.Client
// 	teleportConfig lib.TeleportConfig
// }

var (
	// testAccProviders represents an array of provider tests
	testAccProviders = map[string]*schema.Provider{
		"teleport": provider.Provider(),
	}
	// providerConfig represents provider configuration
	providerConfig string = `
		provider "teleport" {
			addr = "localhost:3025"
			identity_file_path = "/Users/gzigzigzeo/go/src/github.com/gravitational/teleport-plugins/terraform/tmp/identity"
		}
	`
)

func testAccExampleResource(name string) string {
	return providerConfig + `
		resource "teleport_provision_token" "example" {
			metadata {
				name = "test2"
				expires = "2022-10-12T07:20:51.2Z"
				description = "Example token"
			
				labels = {
					example = "yes" 
				}
			}
		
			spec {
				roles = ["Node", "Auth"]
			}
		}	  
	`
}

func testAccExampleResource_removedPolicy(name string) string {
	return providerConfig + `
	`
}

func testAccCheckExampleResourceExists(state *terraform.State) error {
	return nil
}

func TestAccExampleWidget_basic(t *testing.T) {
	//var widgetBefore, widgetAfter types.ProvisionTokenV2
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		//PreCheck:     func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		//CheckDestroy: testAccCheckExampleResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccExampleResource(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckExampleResourceExists,
				),
			},
			{
				Config: testAccExampleResource_removedPolicy(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckExampleResourceExists,
				),
			},
		},
	})
}
