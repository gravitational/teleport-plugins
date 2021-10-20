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

func (s *TerraformSuite) TestToken() {
	res := "teleport_provision_token"

	create := s.terraformConfig + `
		resource "` + res + `" "test" {
			metadata {
				name = "test"
				expires = "2023-01-01T00:00:00Z"
				labels = {
					example = "yes"
				}
			}
			spec {
				roles = ["Node", "Auth"]
			}
		}
	`

	update := s.terraformConfig + `
		resource "` + res + `" "test" {
			metadata {
				name = "test"
				expires = "2023-01-01T00:00:00Z"
			}
			spec {
				roles = ["Node"]
			}
		}
	`

	checkTokenDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetToken(s.Context(), "test")
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := res + ".test"

	resource.Test(s.T(), resource.TestCase{
		Providers:    s.terraformProviders,
		CheckDestroy: checkTokenDestroyed,
		Steps: []resource.TestStep{
			{
				Config: create,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "token"),
					resource.TestCheckResourceAttr(name, "metadata.0.name", "test"),
					resource.TestCheckResourceAttr(name, "metadata.0.expires", "2023-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr(name, "metadata.0.labels.example", "yes"),
					resource.TestCheckResourceAttr(name, "spec.0.roles.0", "Node"),
					resource.TestCheckResourceAttr(name, "spec.0.roles.1", "Auth"),
				),
			},
			{
				Config:   create, // Check that there is no state drift
				PlanOnly: true,
			},
			{
				Config: update,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "token"),
					resource.TestCheckResourceAttr(name, "metadata.0.name", "test"),
					resource.TestCheckResourceAttr(name, "metadata.0.expires", "2023-01-01T00:00:00Z"),
					resource.TestCheckNoResourceAttr(name, "metadata.0.labels.example"),
					resource.TestCheckResourceAttr(name, "spec.0.roles.0", "Node"),
					resource.TestCheckNoResourceAttr(name, "spec.0.roles.1"),
				),
			},
			{
				Config:   update, // Check that there is no state drift
				PlanOnly: true,
			},
		},
	})
}
