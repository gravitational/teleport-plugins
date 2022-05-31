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
	"regexp"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
)

func (s *TerraformSuite) TestSAMLConnector() {
	checkDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetSAMLConnector(s.Context(), "test", false)
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := "teleport_saml_connector.test"

	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		CheckDestroy:             checkDestroyed,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("saml_connector_0_create.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "saml"),
					resource.TestCheckResourceAttr(name, "spec.acs", "https://example.com/v1/webapi/saml/acs"),
					resource.TestCheckResourceAttr(name, "spec.attributes_to_roles.0.name", "groups"),
					resource.TestCheckResourceAttr(name, "spec.attributes_to_roles.0.roles.0", "admin"),
					resource.TestCheckResourceAttr(name, "spec.attributes_to_roles.0.value", "okta-admin"),
				),
			},
			{
				Config:   s.getFixture("saml_connector_0_create.tf"),
				PlanOnly: true,
			},
			{
				Config: s.getFixture("saml_connector_1_update.tf"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "saml"),
					resource.TestCheckResourceAttr(name, "spec.acs", "https://example.com/v1/webapi/saml/acs"),
					resource.TestCheckResourceAttr(name, "spec.attributes_to_roles.0.name", "groups"),
					resource.TestCheckResourceAttr(name, "spec.attributes_to_roles.0.roles.0", "admin"),
					resource.TestCheckResourceAttr(name, "spec.attributes_to_roles.0.value", "okta-admin"),
				),
			},
			{
				Config:   s.getFixture("saml_connector_1_update.tf"),
				PlanOnly: true,
			},
		},
	})
}

func (s *TerraformSuite) TestImportSAMLConnector() {
	r := "teleport_saml_connector"
	id := "test_import"
	name := r + "." + id

	samlConnector := &types.SAMLConnectorV2{
		Metadata: types.Metadata{
			Name: id,
		},
		Spec: types.SAMLConnectorSpecV2{
			AssertionConsumerService: "https://example.com/v1/webapi/saml/acs",
			EntityDescriptor: `
<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="http://www.okta.com/exk1hqp7cwfwMSmWU5d7">
<md:IDPSSODescriptor WantAuthnRequestsSigned="false" protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
<md:KeyDescriptor use="signing">
<ds:KeyInfo xmlns:ds="http://www.w3.org/2000/09/xmldsig#">
<ds:X509Data>
<ds:X509Certificate>---</ds:X509Certificate>
</ds:X509Data>
</ds:KeyInfo>
</md:KeyDescriptor>
<md:NameIDFormat>urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress</md:NameIDFormat>
<md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="https://dev-82418781.okta.com/app/dev-82418781_evilmartiansteleportsh_1/exk1hqp7cwfwMSmWU5d7/sso/saml"/>
<md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://dev-82418781.okta.com/app/dev-82418781_evilmartiansteleportsh_1/exk1hqp7cwfwMSmWU5d7/sso/saml"/>
</md:IDPSSODescriptor>
</md:EntityDescriptor>				
`,
			AttributesToRoles: []types.AttributeMapping{
				{
					Name:  "map attrx to rolex",
					Value: "attrx",
					Roles: []string{"rolex"},
				},
			},
		},
	}

	err := samlConnector.CheckAndSetDefaults()
	require.NoError(s.T(), err)

	err = s.client.UpsertSAMLConnector(s.Context(), samlConnector)
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
					require.Equal(s.T(), state[0].Attributes["kind"], "saml")
					require.Equal(s.T(), state[0].Attributes["spec.acs"], "https://example.com/v1/webapi/saml/acs")

					return nil
				},
			},
		},
	})
}

func (s *TerraformSuite) TestSAMLConnectorWithEntityDescriptorURL() {
	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		Steps: []resource.TestStep{
			{
				Config: s.getFixture("saml_connector_0_create_with_entitydescriptorurl.tf"),
			},
		},
	})
}

func (s *TerraformSuite) TestSAMLConnectorWithoutEntityDescriptor() {
	resource.Test(s.T(), resource.TestCase{
		ProtoV6ProviderFactories: s.terraformProviders,
		Steps: []resource.TestStep{
			{
				Config:      s.getFixture("saml_connector_0_create_without_entitydescriptor.tf"),
				ExpectError: regexp.MustCompile("AnyOf 'entity_descriptor, entity_descriptor_url' keys must be present"),
			},
		},
	})
}
