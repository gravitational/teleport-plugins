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

func (s *TerraformSuite) TestRole() {
	res := "teleport_role"

	create := s.terraformConfig + `
		resource "` + res + `" "test" {
			metadata {
				name        = "test"
				labels = {
					example  = "yes"      
				}
			}
			
			spec {
				options {
					forward_agent           = false
					max_session_ttl         = "7m"
					request_access          = "denied"
				}
			
				allow {
					logins = ["example"]
			
					rules {
						resources = ["user", "role"]
						verbs = ["list"]
					}
				
					node_labels {
						key = "example"
						value = ["yes"]
					}
				}
			
				deny {
					logins = ["anonymous"]
				}
			}
		}
	`

	update := s.terraformConfig + `
		resource "` + res + `" "test" {
			metadata {
				name        = "test"
				labels = {
					example  = "yes"      
				}
			}
			
			spec {
				options {
					forward_agent           = true
					max_session_ttl         = "1h00m"
				}
			
				allow {
					logins = ["example"]
			
					rules {
						resources = ["user", "role"]
						verbs = ["list"]
					}
				
					node_labels {
						key = "example"
						value = ["yes"]
					}

					node_labels {
						key = "additional"
						value = ["yes"]
					}
				}
			
				deny {
					logins = ["anonymous"]
				}
			}
		}
	`

	checkRoleDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetRole(s.Context(), "test")
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := res + ".test"

	resource.Test(s.T(), resource.TestCase{
		ProviderFactories: s.terraformProviders,
		CheckDestroy:      checkRoleDestroyed,
		Steps: []resource.TestStep{
			{
				Config: create,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "role"),
					resource.TestCheckResourceAttr(name, "spec.0.options.0.forward_agent", "false"),
					resource.TestCheckResourceAttr(name, "spec.0.options.0.max_session_ttl", "7m0s"),
					resource.TestCheckResourceAttr(name, "spec.0.options.0.request_access", "denied"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.logins.0", "example"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.rules.0.resources.0", "user"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.rules.0.resources.1", "role"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.rules.0.verbs.0", "list"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.node_labels.0.key", "example"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.node_labels.0.value.0", "yes"),
				),
			},
			{
				Config:   create, // Check that there is no state drift
				PlanOnly: true,
			},
			{
				Config: update,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "role"),
					resource.TestCheckResourceAttr(name, "spec.0.options.0.forward_agent", "true"),
					resource.TestCheckResourceAttr(name, "spec.0.options.0.max_session_ttl", "1h0m0s"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.logins.0", "example"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.rules.0.resources.0", "user"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.rules.0.resources.1", "role"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.rules.0.verbs.0", "list"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.node_labels.0.key", "additional"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.node_labels.0.value.0", "yes"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.node_labels.1.key", "example"),
					resource.TestCheckResourceAttr(name, "spec.0.allow.0.node_labels.1.value.0", "yes"),
				),
			},
			{
				Config:   update, // Check that there is no state drift
				PlanOnly: true,
			},
		},
	})

	s.closeClient()
}
