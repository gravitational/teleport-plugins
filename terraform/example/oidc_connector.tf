# Teleport OIDC connector

resource "teleport_oidc_connector" "example" {
  metadata = {
    name = "example"
    labels = {
      test = "yes"
    }
  }

  spec = {
    client_id = "client"
    client_secret = "value"

    claims_to_roles = [{
      claim = "test"
      roles = ["terraform"]
    }]
  }
}
