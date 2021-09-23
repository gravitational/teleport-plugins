# Teleport Cluster Audit config

resource "teleport_cluster_audit_config" "example" {
   metadata {
    description = "Cluster audit config"
    labels = {
      "example" = "yes"
    }
  }

  spec {
    enable_continuous_backups = true
    audit_events_uri = ["http://example.com"]
  }
}