# Teleport Provision Token resource

resource "teleport_provision_token" "example" {
  metadata = {
    expires = "2022-10-12T07:20:51Z"
    description = "Example token"

    labels = {
      example = "yes" 
      "teleport.dev/origin" = "dynamic" // This label is added on Teleport side by default
    }
  }

  spec = {
    roles = ["Node", "Auth"]
  }
}
