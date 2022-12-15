resource "teleport_auth_preference" "test" {
  metadata = {
    labels = {
      "teleport.dev/origin" = "dynamic"
    }
  }

  spec = {
    disconnect_expired_cert = false
  }
}
