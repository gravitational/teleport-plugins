resource "teleport_provision_token" "test" {
  metadata = {
    name    = "thisisasecretandmustnotbelogged"
    expires = "2038-01-01T00:00:00Z"
    labels = {
      example = "yes"
    }
  }
  spec = {
    roles = ["Node", "Auth"]
  }
}