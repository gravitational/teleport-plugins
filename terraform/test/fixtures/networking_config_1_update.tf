resource "teleport_cluster_networking_config" "test" {
  metadata = {
    labels = {
      "example"             = "no"
      "teleport.dev/origin" = "dynamic"
    }
  }

  spec = {
    client_idle_timeout = "1h"
  }
}
