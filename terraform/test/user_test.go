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
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func (s *TerraformSuite) TestUser() {
	res := "teleport_user"

	create := s.terraformConfig + `
		resource "` + res + `" "test" {
			metadata {
				name    = "test"			
				expires = "2022-10-12T07:20:50.3Z"
				labels  = {
				  	example = "yes"
				}
			}
			
			spec {
				roles = ["admin"]

				traits {
					key   = "logins1"
					value = ["example"]
				}
			
				traits {
					key   = "logins2"
					value = ["example"]
				}

				oidc_identities {
					connector_id = "oidc"
					username     = "example"
				}
						
				github_identities {
					connector_id = "github"
					username     = "example"
				}
			
				saml_identities {
					connector_id = "saml"
					username     = "example"
				}		 
			}
		}
	`

	update := s.terraformConfig + `
		resource "` + res + `" "test" {
			metadata {
				name    = "test"			
				expires = "2022-10-12T07:20:50.3Z"
				labels  = {
				  	example = "yes"
				}
			}
			
			spec {
				roles = ["admin"]
			
				traits {
					key   = "logins2"
					value = ["example"]
				}

				oidc_identities {
					connector_id = "oidc-2"
					username     = "example"
				}						
			}
		}
	`
	checkUserDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetUser("test", false)
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := res + ".test"

	resource.Test(s.T(), resource.TestCase{
		Providers:    s.terraformProviders,
		CheckDestroy: checkUserDestroyed,
		Steps: []resource.TestStep{
			{
				Config: create,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "user"),
					resource.TestCheckResourceAttr(name, "metadata.0.expires", "2022-10-12T07:20:50.3Z"),
					resource.TestCheckResourceAttr(name, "spec.0.roles.0", "admin"),
					resource.TestCheckResourceAttr(name, "spec.0.traits.0.key", "logins1"),
					resource.TestCheckResourceAttr(name, "spec.0.traits.0.value.0", "example"),
					resource.TestCheckResourceAttr(name, "spec.0.traits.1.key", "logins2"),
					resource.TestCheckResourceAttr(name, "spec.0.traits.1.value.0", "example"),
					resource.TestCheckResourceAttr(name, "spec.0.oidc_identities.0.connector_id", "oidc"),
					resource.TestCheckResourceAttr(name, "spec.0.oidc_identities.0.username", "example"),
					resource.TestCheckResourceAttr(name, "spec.0.github_identities.0.connector_id", "github"),
					resource.TestCheckResourceAttr(name, "spec.0.github_identities.0.username", "example"),
					resource.TestCheckResourceAttr(name, "spec.0.saml_identities.0.connector_id", "saml"),
					resource.TestCheckResourceAttr(name, "spec.0.saml_identities.0.username", "example"),
				),
			},
			{
				Config:   create, // Check that there is no state drift
				PlanOnly: true,
			},
			{
				Config: update,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "user"),
					resource.TestCheckResourceAttr(name, "metadata.0.expires", "2022-10-12T07:20:50.3Z"),
					resource.TestCheckResourceAttr(name, "spec.0.roles.0", "admin"),
					resource.TestCheckResourceAttr(name, "spec.0.traits.0.key", "logins2"),
					resource.TestCheckResourceAttr(name, "spec.0.traits.0.value.0", "example"),
					resource.TestCheckNoResourceAttr(name, "spec.0.traits.1.key"),
					resource.TestCheckResourceAttr(name, "spec.0.oidc_identities.0.connector_id", "oidc-2"),
					resource.TestCheckResourceAttr(name, "spec.0.oidc_identities.0.username", "example"),
					resource.TestCheckNoResourceAttr(name, "spec.0.github_identities.0"),
					resource.TestCheckNoResourceAttr(name, "spec.0.saml_identities.0"),
				),
			},
			{
				Config:   update, // Check that there is no state drift
				PlanOnly: true,
			},
		},
	})
}
