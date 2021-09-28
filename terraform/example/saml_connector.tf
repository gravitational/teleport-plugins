# Teleport SAML connector

variable "saml_entity_descriptor" {}

resource "teleport_saml_connector" "example-saml" {
  metadata {
    name = "example-saml"
  }

  spec {
    attributes_to_roles {
      name = "groups"
      roles = ["example"]
      value = "okta-admin"
    }

    attributes_to_roles {
      name = "groups"
      roles = ["example"]
      value = "okta-dev"
    }

    acs = "https://${var.addr}/v1/webapi/saml/acs"
    entity_descriptor = var.saml_entity_descriptor
  }
}