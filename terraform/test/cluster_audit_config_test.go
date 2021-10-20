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
	"regexp"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func (s *TerraformSuite) TestAuditConfig() {
	res := "teleport_cluster_audit_config"

	create := s.terraformConfig + `
		resource "` + res + `" "test" {
			metadata {
				labels = {
					  "example" = "yes"
				}
			}
							
			spec {
				audit_events_uri = ["http://example.com"]
			}			
		}
	`

	resource.Test(s.T(), resource.TestCase{
		Providers: s.terraformProviders,
		Steps: []resource.TestStep{
			{
				Config:      create,
				ExpectError: regexp.MustCompile("not implemented"),
			},
		},
	})
}
