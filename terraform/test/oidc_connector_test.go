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
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/wrappers"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
)

func (s *TerraformSuite) TestOIDCConnector() {
	checkDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetOIDCConnector(s.Context(), "test", false)
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := "teleport_oidc_connector.test"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		CheckDestroy:             checkDestroyed,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("oidc_connector_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "oidc"),
					resource.TestCheckResourceAttr(name, "metadata.expires", "2032-10-12T07:20:50Z"),
					resource.TestCheckResourceAttr(name, "spec.client_id", "client"),
					resource.TestCheckResourceAttr(name, "spec.claims_to_roles.0.claim", "test"),
					resource.TestCheckResourceAttr(name, "spec.claims_to_roles.0.roles.0", "terraform"),
				),
			},
			{
				Config:   s.getFixture("oidc_connector_0_create.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("oidc_connector_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "oidc"),
					resource.TestCheckResourceAttr(name, "metadata.expires", "2032-10-12T07:20:50Z"),
					resource.TestCheckResourceAttr(name, "spec.client_id", "client"),
					resource.TestCheckResourceAttr(name, "spec.claims_to_roles.0.claim", "test"),
					resource.TestCheckResourceAttr(name, "spec.claims_to_roles.0.roles.0", "teleport"),
				),
			},
			{
				Config:   s.getFixture("oidc_connector_1_update.tf"),
				PlanOnly: true,
			},
		},
	})
}

func (s *TerraformSuite) TestImportOIDCConnector() {
	r := "teleport_oidc_connector"
	id := "test_import"
	name := r + "." + id

	oidcConnector := &types.OIDCConnectorV3{
		Metadata: types.Metadata{
			Name: id,
		},
		Spec: types.OIDCConnectorSpecV3{
			ClientID:     "Iv1.3386eee92ff932a4",
			ClientSecret: "secret",
			ClaimsToRoles: []types.ClaimMapping{
				{
					Claim: "test",
					Roles: []string{"terraform"},
				},
			},
			RedirectURLs: wrappers.Strings{
				"https://example.com/redirect",
			},
		},
	}

	err := oidcConnector.CheckAndSetDefaults()
	require.NoError(s.T(), err)

	err = s.client.UpsertOIDCConnector(s.Context(), oidcConnector)
	require.NoError(s.T(), err)

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		Steps: []resource.TestStep{
			{
				Config:        s.terraformConfig + "\n" + `resource "` + r + `" "` + id + `" { }`,
				ResourceName:  name,
				ImportState:   true,
				ImportStateId: id,
				ImportStateCheck: func(state []*terraform.InstanceState) error {
					require.Equal(s.T(), state[0].Attributes["kind"], "oidc")
					require.Equal(s.T(), state[0].Attributes["spec.client_id"], "Iv1.3386eee92ff932a4")
					require.Equal(s.T(), state[0].Attributes["spec.claims_to_roles.0.claim"], "test")
					require.Equal(s.T(), state[0].Attributes["spec.claims_to_roles.0.roles.0"], "terraform")

					return nil
				},
			},
		},
	})
}
