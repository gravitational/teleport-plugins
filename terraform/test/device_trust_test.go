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
	"fmt"

	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func (s *TerraformSuite) TestDeviceTrust() {
	device1 := "teleport_trusted_device.TESTDEVICE1"
	device2 := "teleport_trusted_device.TESTDEVICE2"

	allDevices := []string{device1, device2}

	checkDeviceDestroyed := func(state *terraform.State) error {
		for _, deviceName := range allDevices {
			_, err := s.client.GetDeviceResource(s.Context(), deviceName)
			switch {
			case err == nil:
				return fmt.Errorf("Device %s was not deleted", deviceName)
			case trace.IsNotFound(err):
				continue
			default:
				return err
			}
		}
		return nil
	}

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		CheckDestroy:             checkDeviceDestroyed,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("device_trust_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(device1, "spec.asset_tag", "TESTDEVICE1"),
					resource.TestCheckResourceAttr(device1, "spec.os_type", "macos"),
					resource.TestCheckResourceAttr(device1, "spec.enroll_status", "enrolled"),
				),
			},
			{
				Config:   s.getFixture("device_trust_0_create.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("device_trust_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(device1, "spec.enroll_status", "not_enrolled"),
					resource.TestCheckResourceAttr(device2, "spec.asset_tag", "TESTDEVICE2"),
					resource.TestCheckResourceAttr(device2, "spec.os_type", "linux"),
					resource.TestCheckResourceAttr(device2, "spec.enroll_status", "not_enrolled"),
				),
			},
			{
				Config:   s.getFixture("device_trust_1_update.tf"),
				PlanOnly: true,
			},
		},
	})
}
