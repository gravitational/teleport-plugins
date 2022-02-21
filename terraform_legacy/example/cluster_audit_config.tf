# Teleport Cluster Audit config

# Set is not implemented in the API: https://github.com/gravitational/teleport/blob/1944e62cc55d74d26b337945000b192528674ef3/api/client/client.go#L1776

# resource "teleport_cluster_audit_config" "example" {
#    metadata {
#     description = "Cluster audit config"
#     labels = {
#       "example" = "yes"
#     }
#   }

#   spec {
#     audit_events_uri = ["http://example.com"]
#   }
# }