resource "teleport_provision_token" "test" {
    metadata = {
        name = "test"
        expires = "2028-01-01T00:00:00Z"
        labels = {
            example = "yes"
        }
    }
    spec = {
        roles = ["Node", "Auth"]
    }
}
