resource "teleport_role" "test" {
  metadata = {
    name    = "test"
    expires = "2032-12-12T00:00:00Z"
  }

  spec = {
    allow = {
      logins      = ["anonymous"]
      request     = {}
      node_labels = {}
    }
  }
}
