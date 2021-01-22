terraform {
    required_providers {
      teleport = {
          source = "gravitational.com/teleport/teleport"
          version = "0.0.1"
      }
    }
}

# Teleport Terraform Provider configuration
provider "teleport" {
  addr      = "localhost:3025"
  cert_path = ""
  key_path  = ""
  root_ca_path   = ""
}