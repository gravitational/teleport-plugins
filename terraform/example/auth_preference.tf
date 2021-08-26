# AuthPreference resource

resource "teleport_auth_preference" "example" {
  metadata {
    description = "Auth preference"
    labels = {
      "example" = "yes"
    }
  }

  spec {
    disconnect_expired_cert = true
  }
}
