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

func (s *TerraformSuite) TestApp() {
	res := "teleport_app"

	create := s.terraformConfig + `
		resource "` + res + `" "test" {
            metadata {
                name = "example"
                description = "Test app"
                labels  = {
                    example = "yes"
                    "teleport.dev/origin" = "dynamic"
                }    
            }
        
            spec {
                uri = "localhost:3000"
            }
        }

	`

	update := s.terraformConfig + `
		resource "` + res + `" "test" {
            metadata {
                name = "example"
                description = "Test app"
                labels  = {
                    example = "yes"
                    "teleport.dev/origin" = "dynamic"
                }    
            }
        
            spec {
                uri = "localhost:3000"
            }
		}
	`

	checkAppDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetApp(s.Context(), "test")
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := res + ".test"

	resource.Test(s.T(), resource.TestCase{
		ProviderFactories: s.terraformProviders,
		CheckDestroy:      checkAppDestroyed,
		Steps: []resource.TestStep{
			{
				Config: create,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "app"),
					resource.TestCheckResourceAttr(name, "spec.0.uri", "localhost:3000"),
				),
			},
			{
				Config:   create, // Check that there is no state drift
				PlanOnly: true,
			},
			{
				Config: update,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "app"),
					resource.TestCheckResourceAttr(name, "spec.0.uri", "localhost:3000"),
				),
			},
			{
				Config:   update, // Check that there is no state drift
				PlanOnly: true,
			},
		},
	})
}
