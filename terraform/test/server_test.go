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
	"github.com/gravitational/teleport/api/defaults"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func (s *TerraformSuite) TestServer() {
	checkServerDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetNode(s.Context(), defaults.Namespace, "test")
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := "teleport_server.test"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		CheckDestroy:             checkServerDestroyed,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("server_openssh_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "node"),
					resource.TestCheckResourceAttr(name, "sub_kind", "openssh"),
					resource.TestCheckResourceAttr(name, "version", "v2"),
					resource.TestCheckResourceAttr(name, "spec.addr", "127.0.0.1:22"),
					resource.TestCheckResourceAttr(name, "spec.hostname", "test.local"),
				),
			},
			{
				Config:   s.getFixture("server_openssh_0_create.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("server_openssh_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "node"),
					resource.TestCheckResourceAttr(name, "sub_kind", "openssh"),
					resource.TestCheckResourceAttr(name, "version", "v2"),
					resource.TestCheckResourceAttr(name, "spec.addr", "127.0.0.1:23"),
					resource.TestCheckResourceAttr(name, "spec.hostname", "test.local"),
				),
			},
			{
				Config:   s.getFixture("server_openssh_1_update.tf"),
				PlanOnly: true,
			},
		},
	})
}
