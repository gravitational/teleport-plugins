resource "teleport_provision_token" "test" {
  metadata = {
    name = "test"
    labels = {
      example = "yes"
    }
  }
  spec = {
    roles = ["Node", "Auth"]
  }
}
