resource "teleport_role" "splitbrain" {
  metadata = {
    name = "splitbrain"
  }

  spec = {
    allow = {
      logins = ["one"]
    }
  }

  version = "v6"
}
