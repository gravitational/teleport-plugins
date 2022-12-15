resource "teleport_app" "test" {
  metadata = {
    name        = "test"
    description = "Test app"
    labels = {
      example               = "yes"
      "teleport.dev/origin" = "dynamic"
    }
  }

  spec = {
    uri = "localhost:3000"
  }
}
