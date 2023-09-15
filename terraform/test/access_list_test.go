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
	"context"

	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func (s *TerraformSuite) TestAccessList() {
	checkAccessListDestroyed := func(state *terraform.State) error {
		_, err := s.client.AccessListClient().GetAccessList(context.TODO(), "test")
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := "teleport_access_list.test"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		CheckDestroy:             checkAccessListDestroyed,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("access_list_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "header.metadata.name", "test"),
					resource.TestCheckResourceAttr(name, "spec.description", "test description"),
					resource.TestCheckResourceAttr(name, "spec.owners.0.name", "gru"),
					resource.TestCheckResourceAttr(name, "spec.membership_requires.roles.0", "minion"),
					resource.TestCheckResourceAttr(name, "spec.grants.roles.0", "crane-operator"),
				),
			},
			{
				Config:   s.getFixture("access_list_0_create.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("access_list_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "spec.grants.traits.0.key", "allowed-machines"),
					resource.TestCheckResourceAttr(name, "spec.grants.traits.0.values.0", "crane"),
					resource.TestCheckResourceAttr(name, "spec.grants.traits.0.values.1", "forklift"),
				),
			},
			{
				Config:   s.getFixture("access_list_1_update.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("access_list_2_expiring.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "header.metadata.expires", "2038-01-01T00:00:00Z"),
				),
			},
			{
				Config:   s.getFixture("access_list_2_expiring.tf"),
				PlanOnly: true,
			},
		},
	})
}
