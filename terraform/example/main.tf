variable "identity_file_path" {}
variable "addr" {}

terraform {
  required_providers {
    teleport = {
      version = "8.3.1"
      source  = "gravitational.com/teleport/teleport"
    }
  }
}

# Terraform Provider configuration. See provider.go for available options
provider "teleport" {
  identity_file_path = var.identity_file_path
  addr = var.addr
}