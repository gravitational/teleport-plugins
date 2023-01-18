resource "teleport_database" "test" {
  metadata = {
    name    = "test"
    expires = "2032-10-12T07:20:50Z"
    labels = {
      "teleport.dev/origin" = "dynamic"
      example               = "yes"
    }
  }

  spec = {
    protocol = "postgres"
    uri      = "example.com:5432"
  }
}
