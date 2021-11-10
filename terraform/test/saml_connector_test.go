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
	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func (s *TerraformSuite) TestSAMLConnector() {
	if !s.teleportFeatures.AdvancedAccessWorkflows {
		s.T().Skip("AdvancedAccessWorkflows are disabled")
	}

	res := "teleport_saml_connector"

	create := s.terraformConfig + `
		resource "` + res + `" "test" {
			metadata {
				name    = "test"
				expires = "2022-10-12T07:20:50.3Z"
				labels  = {
				  	example = "yes"
				}
			}

			spec {
				attributes_to_roles {
					name = "groups"
					roles = ["admin"]
					value = "okta-admin"
				}
				
				acs = "https://example.com/v1/webapi/saml/acs"
				entity_descriptor = <<EOT
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
EOT
			}
		}
	`

	update := s.terraformConfig + `
		resource "` + res + `" "test" {
			metadata {
				name    = "test"
				expires = "2022-10-12T07:20:50.3Z"
				labels  = {
					example = "no"
				}
			}

			spec {
				attributes_to_roles {
					name = "groups"
					roles = ["admin"]
					value = "okta-admin"
				}
								
				acs = "https://example.com/v1/webapi/saml/acs"
				entity_descriptor = <<EOT
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
EOT
			}
		}
	`

	checkSAMLConnectorDestroyed := func(state *terraform.State) error {
		_, err := s.client.GetSAMLConnector(s.Context(), "test", true)
		if trace.IsNotFound(err) {
			return nil
		}

		return err
	}

	name := res + ".test"

	resource.Test(s.T(), resource.TestCase{
		ProviderFactories: s.terraformProviders,
		CheckDestroy:      checkSAMLConnectorDestroyed,
		Steps: []resource.TestStep{
			{
				Config: create,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "saml"),
					resource.TestCheckResourceAttr(name, "spec.0.acs", "https://example.com/v1/webapi/saml/acs"),
					resource.TestCheckResourceAttr(name, "spec.0.attributes_to_roles.0.name", "groups"),
					resource.TestCheckResourceAttr(name, "spec.0.attributes_to_roles.0.roles.0", "admin"),
					resource.TestCheckResourceAttr(name, "spec.0.attributes_to_roles.0.value", "okta-admin"),
				),
			},
			{
				Config:   create, // Check that there is no state drift
				PlanOnly: true,
			},
			{
				Config: update,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "kind", "saml"),
					resource.TestCheckResourceAttr(name, "spec.0.acs", "https://example.com/v1/webapi/saml/acs"),
					resource.TestCheckResourceAttr(name, "spec.0.attributes_to_roles.0.name", "groups"),
					resource.TestCheckResourceAttr(name, "spec.0.attributes_to_roles.0.roles.0", "admin"),
					resource.TestCheckResourceAttr(name, "spec.0.attributes_to_roles.0.value", "okta-admin"),
				),
			},
			{
				Config:   update, // Check that there is no state drift
				PlanOnly: true,
			},
		},
	})
}
