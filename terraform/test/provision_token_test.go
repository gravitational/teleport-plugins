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
	"fmt"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
)

func (s *TerraformSuite) TestProvisionToken() {
	checkRoleDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetToken(s.Context(), "test")
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := "teleport_provision_token.test"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		CheckDestroy:             checkRoleDestroyed,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("provision_token_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "token"),
					resource.TestCheckResourceAttr(name, "metadata.name", "test"),
					resource.TestCheckResourceAttr(name, "metadata.expires", "2038-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr(name, "metadata.labels.example", "yes"),
					resource.TestCheckResourceAttr(name, "spec.roles.0", "Node"),
					resource.TestCheckResourceAttr(name, "spec.roles.1", "Auth"),

					resource.TestCheckResourceAttr(name, "version", "v2"),
				),
			},
			{
				Config:   s.getFixture("provision_token_0_create.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("provision_token_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "token"),
					resource.TestCheckResourceAttr(name, "metadata.name", "test"),
					resource.TestCheckResourceAttr(name, "metadata.expires", "2038-01-01T00:00:00Z"),
					resource.TestCheckNoResourceAttr(name, "metadata.labels.example"),
					resource.TestCheckResourceAttr(name, "spec.roles.0", "Node"),
					resource.TestCheckNoResourceAttr(name, "spec.roles.1"),

					resource.TestCheckResourceAttr(name, "version", "v2"),
				),
			},
			{
				Config:   s.getFixture("provision_token_1_update.tf"),
				PlanOnly: true,
			},
		},
	})
}

func (s *TerraformSuite) TestProvisionTokenV2() {
	checkRoleDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetToken(s.Context(), "test")
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := "teleport_provision_token.test2"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		CheckDestroy:             checkRoleDestroyed,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("provision_token_v2_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "token"),
					resource.TestCheckResourceAttr(name, "metadata.name", "test2"),
					resource.TestCheckResourceAttr(name, "metadata.expires", "2038-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr(name, "metadata.labels.example", "yes"),
					resource.TestCheckResourceAttr(name, "spec.roles.0", "Node"),
					resource.TestCheckResourceAttr(name, "spec.roles.1", "Auth"),
					resource.TestCheckResourceAttr(name, "spec.join_method", "iam"),
					resource.TestCheckResourceAttr(name, "spec.allow.0.aws_account", "1234567890"),

					resource.TestCheckResourceAttr(name, "version", "v2"),
				),
			},
			{
				Config:   s.getFixture("provision_token_v2_0_create.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("provision_token_v2_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "token"),
					resource.TestCheckResourceAttr(name, "metadata.name", "test2"),
					resource.TestCheckResourceAttr(name, "metadata.expires", "2038-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr(name, "metadata.labels.example", "yes"),
					resource.TestCheckResourceAttr(name, "spec.roles.0", "Node"),
					resource.TestCheckResourceAttr(name, "spec.roles.1", "Auth"),
					resource.TestCheckResourceAttr(name, "spec.join_method", "iam"),
					resource.TestCheckResourceAttr(name, "spec.allow.0.aws_account", "1234567890"),
					resource.TestCheckResourceAttr(name, "spec.allow.1.aws_account", "1111111111"),

					resource.TestCheckResourceAttr(name, "version", "v2"),
				),
			},
			{
				Config:   s.getFixture("provision_token_v2_1_update.tf"),
				PlanOnly: true,
			},
		},
	})
}

func (s *TerraformSuite) TestImportProvisionToken() {
	r := "teleport_provision_token"
	id := "test_import"
	name := r + "." + id

	token := &types.ProvisionTokenV2{
		Metadata: types.Metadata{
			Name: id,
		},
		Spec: types.ProvisionTokenSpecV2{
			Roles: []types.SystemRole{"Node", "Auth"},
		},
	}
	err := token.CheckAndSetDefaults()
	require.NoError(s.T(), err)

	err = s.client.UpsertToken(s.Context(), token)
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
					require.Equal(s.T(), state[0].Attributes["kind"], "token")
					require.Equal(s.T(), state[0].Attributes["metadata.name"], "test_import")

					return nil
				},
			},
		},
	})
}

func (s *TerraformSuite) TestProvisionTokenDoesNotLeakSensitiveData() {
	checkRoleDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetToken(s.Context(), "test")
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := "teleport_provision_token.test"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		CheckDestroy:             checkRoleDestroyed,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("provision_token_secret_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "token"),
					resource.TestCheckResourceAttr(name, "metadata.name", "thisisasecretandmustnotbelogged"),
					resource.TestCheckResourceAttr(name, "metadata.expires", "2038-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr(name, "metadata.labels.example", "yes"),
					resource.TestCheckResourceAttr(name, "spec.roles.0", "Node"),
					resource.TestCheckResourceAttr(name, "spec.roles.1", "Auth"),
					resource.TestCheckResourceAttr(name, "version", "v2"),
					func(s *terraform.State) error {
						tokenResource := s.RootModule().Resources[name]
						tokenID := tokenResource.Primary.Attributes["id"]
						tokenName := tokenResource.Primary.Attributes["metadata.name"]
						if tokenID == tokenName {
							return fmt.Errorf("token id must not include the name because the name is the actual token secret")
						}

						return nil
					},
				),
			},
			{
				Config:   s.getFixture("provision_token_secret_0_create.tf"),
				PlanOnly: true,
			},
		},
	})
}
