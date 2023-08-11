/*
Copyright 2023 Gravitational, Inc.

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
	"time"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
)

func (s *TerraformSuite) TestNetworkRestrictions() {
	name := "teleport_network_restrictions.test"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories:  s.terraformProviders,
		PreventPostDestroyRefresh: true,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("network_restrictions_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "network_restrictions"),
					resource.TestCheckResourceAttr(name, "spec.allow.0.cidr", "127.0.0.0/8"),
					resource.TestCheckResourceAttr(name, "spec.deny.0.cidr", "10.1.2.4"),
				),
			},
			{
				Config:   s.getFixture("network_restrictions_0_create.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("network_restrictions_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "network_restrictions"),
					resource.TestCheckResourceAttr(name, "spec.allow.0.cidr", "192.168.0.0/16"),
					resource.TestCheckResourceAttr(name, "spec.deny.0.cidr", "101.101.2.4"),
				),
			},
			{
				Config:   s.getFixture("network_restrictions_1_update.tf"),
				PlanOnly: true,
			},
		},
	})
}

func (s *TerraformSuite) TestImportNetworkRestrictions() {
	r := "teleport_network_restrictions"
	id := "test_import"
	name := r + "." + id

	networkRestrictions := &types.NetworkRestrictionsV4{
		Metadata: types.Metadata{},
		Spec: types.NetworkRestrictionsSpecV4{
			Allow: []types.AddressCondition{{
				CIDR: "127.0.0.0/8",
			}},
		},
	}
	err := networkRestrictions.CheckAndSetDefaults()
	require.NoError(s.T(), err)

	err = s.client.SetNetworkRestrictions(s.Context(), networkRestrictions)
	require.NoError(s.T(), err)

	require.Eventually(s.T(), func() bool {
		_, err := s.client.GetNetworkRestrictions(s.Context())
		if err != nil {
			if !trace.IsNotFound(err) {
				require.NoError(s.T(), err)
			}
			return false
		}

		return true
	}, 5*time.Second, time.Second)

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		Steps: []resource.TestStep{
			{
				Config:        s.terraformConfig + "\n" + `resource "` + r + `" "` + id + `" { }`,
				ResourceName:  name,
				ImportState:   true,
				ImportStateId: id,
				ImportStateCheck: func(state []*terraform.InstanceState) error {
					require.Equal(s.T(), state[0].Attributes["kind"], "network_restrictions")
					require.Equal(s.T(), state[0].Attributes["spec.allow.0.cidr"], "127.0.0.0/8")

					return nil
				},
			},
		},
	})
}
