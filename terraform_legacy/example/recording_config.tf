# Teleport session recording config

resource "teleport_session_recording_config" "example" {
   metadata {
    description = "Session recording config"
    labels = {
      "example" = "yes"
    }
  }

  spec {
    proxy_checks_host_keys = true
  }
}