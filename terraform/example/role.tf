# Teleport Role resource

resource "teleport_role" "example" {
  metadata {
    name        = "example"
    description = "Example Teleport Role"
    expires     = "2022-10-12T07:20:51.2Z"
    labels = {
      example  = "yes"      
    }
  }
  
  spec {
    options {
      forward_agent           = false
      max_session_ttl         = "7m"
      port_forwarding         = false
      client_idle_timeout     = "1h"
      disconnect_expired_cert = true
      permit_x11forwarding    = false
      request_access          = "denied"
    }

    allow {
      logins = ["example"]

      rules {
        resources = ["user", "role"]
        verbs = ["list"]
      }

      request {
        roles = ["example"]
        claims_to_roles {
          claim = "example"
          value = "example"
          roles = ["example"]
        }
      }

      node_labels {
         key = "example"
         value = ["yes"]
      }
    }

    deny {
      logins = ["anonymous"]
    }
  }
}