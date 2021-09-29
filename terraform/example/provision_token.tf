# Teleport Provision Token resource

resource "teleport_provision_token" "example" {
  metadata {
    name = "example"
    expires = "2022-10-12T07:20:51.2Z" # Required
    description = "Example token"

    labels = {
      example = "yes" 
    }
  }

  spec {
    roles = ["Node", "Auth"]
  }
}
