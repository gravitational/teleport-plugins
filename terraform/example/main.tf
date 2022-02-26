variable "identity_file_path" {}
variable "addr" {}

terraform {
  required_providers {
    teleport = {
      version = "8.2.0"
      source  = "gravitational.com/teleport/teleport"
    }
  }
}

# Terraform Provider configuration. See provider.go for available options
provider "teleport" {
  identity_file_path = var.identity_file_path
  addr = var.addr
}

#resource "teleport_provision_token" "example" {
#  metadata = {
#    name = "example"
#    expires = "2022-10-12T07:20:51Z"
#    description = "Token"

#    labels = {
#      example1 = "yes"
#      example2 = "no" 
#      example3 = "unknown"
#    }
#  }

#  spec = {
#    roles = ["Node", "Auth"]
#  }
#}

resource "teleport_app" "test" {
    metadata = {
        name = "example"
        description = "Test app"
        labels  = {
            example = "yes"
            "teleport.dev/origin" = "dynamic"
        }    
    }

    spec = {
        uri = "localhost:3000"
    }

    version = "v3"
}

# resource "teleport_role" "test" {
#     metadata = {
#         name = "test"
#         description = "Test role"
#         expires = "2022-12-12T00:00:00Z"
#     }

#     spec = {
#         options = {
#             max_session_ttl = "2s"
#         }
#         allow = {
#             logins = ["anonymous"]
#             request = {
#                 roles = ["example", "terraform"]
#                 claims_to_roles = [
#                     {
#                         claim = "example"
#                         value = "example"
#                         roles = ["example"]
#                     },
#                 ]
#             }

#             node_labels = {
#                 "example" = ["no"]
#                 "sample" = ["yes", "no"]
#             }            
#         }
#     }

#     version = "v4"
# }
