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
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
	"time"
)

func (s *TerraformSuite) TestOpenSSHServer() {
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
					resource.TestCheckResourceAttr(name, "kind", types.KindNode),
					resource.TestCheckResourceAttr(name, "sub_kind", types.SubKindOpenSSHNode),
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
					resource.TestCheckResourceAttr(name, "kind", types.KindNode),
					resource.TestCheckResourceAttr(name, "sub_kind", types.SubKindOpenSSHNode),
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

func (s *TerraformSuite) TestOpenSSHServerNameless() {
	checkServerDestroyed := func(state *terraform.State) error {
		// The name is a UUID but we can lookup by hostname as well.
		_, err := s.client.GetNode(s.Context(), defaults.Namespace, "test.local")
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
				Config: s.getFixture("server_openssh_nameless_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", types.KindNode),
					resource.TestCheckResourceAttr(name, "sub_kind", types.SubKindOpenSSHNode),
					resource.TestCheckResourceAttr(name, "version", "v2"),
					resource.TestCheckResourceAttr(name, "spec.addr", "127.0.0.1:22"),
					resource.TestCheckResourceAttr(name, "spec.hostname", "test.local"),
				),
			},
			{
				Config:   s.getFixture("server_openssh_nameless_0_create.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("server_openssh_nameless_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", types.KindNode),
					resource.TestCheckResourceAttr(name, "sub_kind", types.SubKindOpenSSHNode),
					resource.TestCheckResourceAttr(name, "version", "v2"),
					resource.TestCheckResourceAttr(name, "spec.addr", "127.0.0.1:23"),
					resource.TestCheckResourceAttr(name, "spec.hostname", "test.local"),
				),
			},
			{
				Config:   s.getFixture("server_openssh_nameless_1_update.tf"),
				PlanOnly: true,
			},
		},
	})
}

func (s *TerraformSuite) TestImportOpenSSHServer() {
	r := "teleport_server"
	id := "test_import"
	name := r + "." + id

	server := &types.ServerV2{
		Kind:    types.KindNode,
		SubKind: types.SubKindOpenSSHNode,
		Version: types.V2,
		Metadata: types.Metadata{
			Name: id,
		},
		Spec: types.ServerSpecV2{
			Addr:     "127.0.0.1:22",
			Hostname: "foobar",
		},
	}
	err := server.CheckAndSetDefaults()
	require.NoError(s.T(), err)

	_, err = s.client.UpsertNode(s.Context(), server)
	require.NoError(s.T(), err)

	require.Eventually(s.T(), func() bool {
		_, err = s.client.GetNode(s.Context(), defaults.Namespace, server.GetName())
		if trace.IsNotFound(err) {
			return false
		}
		require.NoError(s.T(), err)
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
					require.Equal(s.T(), state[0].Attributes["kind"], types.KindNode)
					require.Equal(s.T(), state[0].Attributes["sub_kind"], types.SubKindOpenSSHNode)
					require.Equal(s.T(), state[0].Attributes["spec.addr"], "127.0.0.1:22")
					require.Equal(s.T(), state[0].Attributes["spec.hostname"], "foobar")

					return nil
				},
			},
		},
	})
}

func (s *TerraformSuite) TestOpenSSHEICEServer() {
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
				Config: s.getFixture("server_openssheice_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", types.KindNode),
					resource.TestCheckResourceAttr(name, "sub_kind", types.SubKindOpenSSHEICENode),
					resource.TestCheckResourceAttr(name, "version", "v2"),
					resource.TestCheckResourceAttr(name, "spec.addr", "127.0.0.1:22"),
					resource.TestCheckResourceAttr(name, "spec.hostname", "test.local"),
					resource.TestCheckResourceAttr(name, "spec.cloud_metadata.aws.account_id", "123"),
					resource.TestCheckResourceAttr(name, "spec.cloud_metadata.aws.instance_id", "123"),
					resource.TestCheckResourceAttr(name, "spec.cloud_metadata.aws.region", "us-east-1"),
					resource.TestCheckResourceAttr(name, "spec.cloud_metadata.aws.vpc_id", "123"),
					resource.TestCheckResourceAttr(name, "spec.cloud_metadata.aws.integration", "foo"),
					resource.TestCheckResourceAttr(name, "spec.cloud_metadata.aws.subnet_id", "123"),
				),
			},
			{
				Config:   s.getFixture("server_openssheice_0_create.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("server_openssheice_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", types.KindNode),
					resource.TestCheckResourceAttr(name, "sub_kind", types.SubKindOpenSSHEICENode),
					resource.TestCheckResourceAttr(name, "version", "v2"),
					resource.TestCheckResourceAttr(name, "spec.addr", "127.0.0.1:23"),
					resource.TestCheckResourceAttr(name, "spec.hostname", "test.local"),
					resource.TestCheckResourceAttr(name, "spec.cloud_metadata.aws.account_id", "123"),
					resource.TestCheckResourceAttr(name, "spec.cloud_metadata.aws.instance_id", "123"),
					resource.TestCheckResourceAttr(name, "spec.cloud_metadata.aws.region", "us-east-1"),
					resource.TestCheckResourceAttr(name, "spec.cloud_metadata.aws.vpc_id", "123"),
					resource.TestCheckResourceAttr(name, "spec.cloud_metadata.aws.integration", "foo"),
					resource.TestCheckResourceAttr(name, "spec.cloud_metadata.aws.subnet_id", "123"),
				),
			},
			{
				Config:   s.getFixture("server_openssheice_1_update.tf"),
				PlanOnly: true,
			},
		},
	})
}

func (s *TerraformSuite) TestImportOpenSSHEICEServer() {
	r := "teleport_server"
	id := "test_import"
	name := r + "." + id

	server := &types.ServerV2{
		Kind:    types.KindNode,
		SubKind: types.SubKindOpenSSHEICENode,
		Version: types.V2,
		Metadata: types.Metadata{
			Name: id,
		},
		Spec: types.ServerSpecV2{
			Addr:     "127.0.0.1:22",
			Hostname: "foobar",
			CloudMetadata: &types.CloudMetadata{
				AWS: &types.AWSInfo{
					AccountID:   "123",
					InstanceID:  "123",
					Region:      "us-east-1",
					VPCID:       "123",
					Integration: "foo",
					SubnetID:    "123",
				},
			},
		},
	}
	err := server.CheckAndSetDefaults()
	require.NoError(s.T(), err)

	_, err = s.client.UpsertNode(s.Context(), server)
	require.NoError(s.T(), err)

	require.Eventually(s.T(), func() bool {
		_, err = s.client.GetNode(s.Context(), defaults.Namespace, server.GetName())
		if trace.IsNotFound(err) {
			return false
		}
		require.NoError(s.T(), err)
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
					require.Equal(s.T(), state[0].Attributes["kind"], types.KindNode)
					require.Equal(s.T(), state[0].Attributes["sub_kind"], types.SubKindOpenSSHEICENode)
					require.Equal(s.T(), state[0].Attributes["spec.addr"], "127.0.0.1:22")
					require.Equal(s.T(), state[0].Attributes["spec.hostname"], "foobar")
					require.Equal(s.T(), state[0].Attributes["spec.cloud_metadata.aws.account_id"], "123")
					require.Equal(s.T(), state[0].Attributes["spec.cloud_metadata.aws.instance_id"], "123")
					require.Equal(s.T(), state[0].Attributes["spec.cloud_metadata.aws.region"], "us-east-1")
					require.Equal(s.T(), state[0].Attributes["spec.cloud_metadata.aws.vpc_id"], "123")
					require.Equal(s.T(), state[0].Attributes["spec.cloud_metadata.aws.integration"], "foo")
					require.Equal(s.T(), state[0].Attributes["spec.cloud_metadata.aws.subnet_id"], "123")
					return nil
				},
			},
		},
	})
}
