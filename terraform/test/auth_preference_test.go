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
	"time"

	"github.com/gravitational/teleport/api/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
)

func (s *TerraformSuite) TestAuthPreference() {
	name := "teleport_auth_preference.test"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories:  s.terraformProviders,
		PreventPostDestroyRefresh: true,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("auth_preference_0_set.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "cluster_auth_preference"),
					resource.TestCheckResourceAttr(name, "metadata.labels.example", "yes"),
					resource.TestCheckResourceAttr(name, "spec.disconnect_expired_cert", "true"),
				),
			},
			{
				Config:   s.getFixture("auth_preference_0_set.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("auth_preference_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "cluster_auth_preference"),
					resource.TestCheckResourceAttr(name, "spec.disconnect_expired_cert", "false"),
				),
			},
			{
				Config:   s.getFixture("auth_preference_1_update.tf"),
				PlanOnly: true,
			},
		},
	})
}

func (s *TerraformSuite) TestImportAuthPreference() {
	r := "teleport_auth_preference"
	id := "test_import"
	name := r + "." + id

	authPreference := &types.AuthPreferenceV2{
		Metadata: types.Metadata{},
		Spec: types.AuthPreferenceSpecV2{
			DisconnectExpiredCert: types.NewBoolOption(true),
		},
	}
	err := authPreference.CheckAndSetDefaults()
	require.NoError(s.T(), err)

	authPreferencesBefore, err := s.client.GetAuthPreference(s.Context())
	require.NoError(s.T(), err)

	err = s.client.SetAuthPreference(s.Context(), authPreference)
	require.NoError(s.T(), err)

	require.Eventually(s.T(), func() bool {
		authPreferencesCurrent, err := s.client.GetAuthPreference(s.Context())
		require.NoError(s.T(), err)

		return authPreferencesBefore.GetMetadata().ID != authPreferencesCurrent.GetMetadata().ID
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
					require.Equal(s.T(), state[0].Attributes["kind"], "cluster_auth_preference")
					require.Equal(s.T(), state[0].Attributes["spec.disconnect_expired_cert"], "true")

					return nil
				},
			},
		},
	})
}

func (s *TerraformSuiteWithCache) TestAuthPreferenceAddLabel() {
	name := "teleport_auth_preference.cluster_auth_preference"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories:  s.terraformProviders,
		PreventPostDestroyRefresh: true,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("auth_preference_0_cluster.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "cluster_auth_preference"),
					resource.TestCheckNoResourceAttr(name, "metadata.labels"),
					resource.TestCheckResourceAttr(name, "spec.type", "oidc"),
				),
			},
			// {
			// 	Config:   s.getFixture("auth_preference_0_cluster.tf"),
			// 	PlanOnly: true,
			// },
			// {
			// 	Config: s.getFixture("auth_preference_1_cluster.tf"),
			// 	Check: resource.ComposeTestCheckFunc(
			// 		resource.TestCheckResourceAttr(name, "kind", "cluster_auth_preference"),
			// 		resource.TestCheckResourceAttr(name, "metadata.labels.provisioner", "terraform"),
			// 		resource.TestCheckResourceAttr(name, "spec.type", "oidc"),
			// 	),
			// },
			// {
			// 	Config:   s.getFixture("auth_preference_1_cluster.tf"),
			// 	PlanOnly: true,
			// },
		},
	})
}
