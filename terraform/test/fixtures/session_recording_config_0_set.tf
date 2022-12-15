resource "teleport_session_recording_config" "test" {
  metadata = {
    labels = {
      "example"             = "yes"
      "teleport.dev/origin" = "dynamic"
    }
  }

  spec = {
    mode                   = "node"
    proxy_checks_host_keys = true
  }
}