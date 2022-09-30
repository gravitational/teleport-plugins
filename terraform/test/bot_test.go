/*
Copyright 2022 Gravitational, Inc.

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

func (s *TerraformSuite) TestBot() {
	checkResourcesDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetToken(s.Context(), "bot-test")
		if trace.IsNotFound(err) {
			return nil
		}

		// TODO: won't this be nil?
		return err
	}

	token_name := "teleport_provision_token.bot_test"
	bot_name := "teleport_bot.test"
	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		CheckDestroy:             checkResourcesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("bot_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(token_name, "kind", "token"),
					resource.TestCheckResourceAttr(token_name, "metadata.name", "bot-test"),
					resource.TestCheckResourceAttr(token_name, "spec.roles.0", "Bot"),
					resource.TestCheckResourceAttr(bot_name, "name", "test"),
					resource.TestCheckResourceAttr(bot_name, "user_name", "bot-test"),
					resource.TestCheckResourceAttr(bot_name, "role_name", "bot-test"),
					resource.TestCheckResourceAttr(bot_name, "token_id", "bot-test"),
					resource.TestCheckResourceAttr(bot_name, "roles.0", "terraform"),
					resource.TestCheckNoResourceAttr(bot_name, "spec.traits.logins1"),
				),
			},
			{
				Config:   s.getFixture("bot_0_create.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("bot_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(token_name, "kind", "token"),
					resource.TestCheckResourceAttr(token_name, "metadata.name", "bot-test"),
					resource.TestCheckResourceAttr(token_name, "spec.roles.0", "Bot"),
					resource.TestCheckResourceAttr(bot_name, "name", "test"),
					resource.TestCheckResourceAttr(bot_name, "user_name", "bot-test"),
					resource.TestCheckResourceAttr(bot_name, "role_name", "bot-test"),
					resource.TestCheckResourceAttr(bot_name, "token_id", "bot-test"),
					resource.TestCheckResourceAttr(bot_name, "roles.0", "terraform"),

					// Note: traits are immutable and the plan will not converge
					// if the resource is not recreated when traits are
					// modified.
					resource.TestCheckResourceAttr(bot_name, "traits.logins1.0", "example"),
				),
			},
			{
				Config:   s.getFixture("bot_1_update.tf"),
				PlanOnly: true,
			},
		},
	})

}
