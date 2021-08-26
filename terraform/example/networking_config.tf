# Teleport Cluster Networking config

resource "teleport_cluster_networking_config" "example" {
   metadata {
    description = "Networking config"
    labels = {
      "example" = "yes"
    }
  }

  spec {
    client_idle_timeout = "1h"
  }
}