/*
Copyright 2015-2023 Gravitational, Inc.

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

func (s *TerraformSuite) TestTrustedDevice() {
	checkDeviceDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetDeviceResource(s.Context(), "TESTDEVICE")
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := "teleport_trusted_device.TESTDEVICE"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		CheckDestroy:             checkDeviceDestroyed,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("trusted_device_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "spec.asset_tag", "TESTDEVICE"),
					resource.TestCheckResourceAttr(name, "spec.os_type", "macos"),
					resource.TestCheckResourceAttr(name, "spec.enroll_status", "not_enrolled"),
				),
			},
			{
				Config:   s.getFixture("trusted_device_0_create.tf"),
				PlanOnly: true,
			},
		},
	})
}
