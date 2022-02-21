# Teleport User resource

resource "teleport_user" "example" {
  # Tells Terraform that the role could not be destroyed while this user exists
  depends_on = [
    teleport_role.example
  ]

  metadata {
    name        = "example"
    description = "Example Teleport User"

    expires = "2022-10-12T07:20:50.3Z"

    labels = {
      example = "yes"
    }
  }

  spec {
    roles = ["example"]

    oidc_identities {
      connector_id = "oidc1"
      username     = "example"
    }

    traits {
      key   = "logins1"
      value = ["example"]
    }

    traits {
      key   = "logins2"
      value = ["example"]
    }

    github_identities {
      connector_id = "github"
      username     = "example"
    }

    saml_identities {
      connector_id = "example-saml"
      username     = "example"
    }
  }
}