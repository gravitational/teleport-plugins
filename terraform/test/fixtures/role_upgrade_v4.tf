resource "teleport_role" "upgrade" {
  metadata = {
    name = "upgrade"
  }

  spec = {
    allow = {
      logins = ["onev4"]
    }
  }

  version = "v4"
}
