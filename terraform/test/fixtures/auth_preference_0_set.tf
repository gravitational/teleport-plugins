resource "teleport_auth_preference" "test" {
  metadata = {
    labels = {
      "example"             = "yes"
      "teleport.dev/origin" = "dynamic"
    }
  }

  spec = {
    disconnect_expired_cert = true
  }
}
