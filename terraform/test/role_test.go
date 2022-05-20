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
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
)

func (s *TerraformSuite) TestRole() {
	checkDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetRole(s.Context(), "test")
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := "teleport_role.test"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		CheckDestroy:             checkDestroyed,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("role_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "role"),
					resource.TestCheckNoResourceAttr(name, "spec.options"),
					resource.TestCheckResourceAttr(name, "version", "v4"),
					resource.TestCheckResourceAttr(name, "spec.allow.logins.0", "anonymous"),
				),
			},
			{
				Config:   s.getFixture("role_0_create.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("role_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "role"),
					resource.TestCheckResourceAttr(name, "spec.options.forward_agent", "true"),
					resource.TestCheckResourceAttr(name, "spec.options.max_session_ttl", "2h3m"),
					resource.TestCheckResourceAttr(name, "spec.allow.logins.0", "known"),
					resource.TestCheckResourceAttr(name, "spec.allow.logins.1", "anonymous"),
					resource.TestCheckResourceAttr(name, "spec.allow.request.roles.0", "example"),
					resource.TestCheckResourceAttr(name, "spec.allow.request.claims_to_roles.0.claim", "example"),
					resource.TestCheckResourceAttr(name, "spec.allow.request.claims_to_roles.0.value", "example"),
					resource.TestCheckResourceAttr(name, "spec.allow.request.claims_to_roles.0.roles.0", "example"),
					resource.TestCheckResourceAttr(name, "spec.allow.node_labels.example.0", "yes"),
					resource.TestCheckResourceAttr(name, "spec.allow.node_labels.example.1", "no"),

					resource.TestCheckResourceAttr(name, "version", "v4"),
				),
			},
			{
				Config:   s.getFixture("role_1_update.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("role_2_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "role"),
					resource.TestCheckNoResourceAttr(name, "spec.options"),
					resource.TestCheckResourceAttr(name, "spec.allow.node_labels.example.0", "no"),
					resource.TestCheckResourceAttr(name, "spec.allow.node_labels.sample.0", "yes"),
					resource.TestCheckResourceAttr(name, "spec.allow.node_labels.sample.1", "no"),
				),
			},
			{
				Config:   s.getFixture("role_2_update.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("role_3_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "role"),
					resource.TestCheckNoResourceAttr(name, "spec.options"),
				),
			},
			{
				Config:   s.getFixture("role_3_update.tf"), // Check that there is no state drift
				PlanOnly: true,
			},
		},
	})
}

func (s *TerraformSuiteWithCache) TestRoleMultipleReviewers() {
	checkDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetRole(s.Context(), "test_multiple_reviewers")
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := "teleport_role.test_decrease_reviewers"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		CheckDestroy:             checkDestroyed,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("role_reviewers_0_two_roles.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "role"),
					resource.TestCheckNoResourceAttr(name, "spec.options"),
					resource.TestCheckResourceAttr(name, "spec.allow.review_requests.roles.0", "rolea"),
					resource.TestCheckResourceAttr(name, "spec.allow.review_requests.roles.1", "roleb"),
				),
			},
			{
				Config: s.getFixture("role_reviewers_1_one_role.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "role"),
					resource.TestCheckNoResourceAttr(name, "spec.options"),
					resource.TestCheckResourceAttr(name, "spec.allow.review_requests.roles.0", "roleb"),
					resource.TestCheckNoResourceAttr(name, "spec.allow.review_requests.roles.1"),
				),
			},
		},
	})
}

func (s *TerraformSuite) TestImportRole() {
	r := "teleport_role"
	id := "test_import"
	name := r + "." + id

	role := &types.RoleV5{
		Metadata: types.Metadata{
			Name: id,
		},
		Spec: types.RoleSpecV5{},
	}
	err := role.CheckAndSetDefaults()
	require.NoError(s.T(), err)

	err = s.client.UpsertRole(s.Context(), role)
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
					require.Equal(s.T(), state[0].Attributes["kind"], "role")
					require.Equal(s.T(), state[0].Attributes["metadata.name"], "test_import")

					return nil
				},
			},
		},
	})
}
