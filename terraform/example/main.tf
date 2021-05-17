variable "identity_file_path" {}

terraform {
  required_providers {
    teleport = {
      version = "0.0.1"
      source  = "gravitational.com/teleport/teleport"
    }
  }
}

provider "teleport" {
  identity_file_path = var.identity_file_path

  addr = "localhost:3025"
  # Update addr to point to Teleport Auth/Proxy
  # addr = "evilmartians.teleport.sh:443"
}

resource "teleport_provision_token" "example" {
  metadata {
    name = "example"
    expires = "2022-10-12T07:20:51.2Z"
    description = "Example token"

    labels = {
      example = "yes" 
    }
  }

  spec {
    roles = ["Node", "Auth"]
  }
}

resource "teleport_user" "example" {
  metadata {
    name        = "example"
    description = "Example Teleport User"

    expires = "2022-10-12T07:20:50.3Z"

    labels = {
      example = "yes"
    }
  }

  spec {
    roles = ["example"]

    oidc_identities {
      connector_id = "oidc1"
      username     = "example"
    }

    oidc_identities {
      connector_id = "oidc2"
      username     = "example"
    }

    traits {
      key   = "logins1"
      value = ["example"]
    }

    traits {
      key   = "logins2"
      value = ["example"]
    }

    github_identities {
      connector_id = "github"
      username     = "example"
    }
  }
}

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

# resource "teleport_github_connector" "github" {
#   metadata {
#      name = "test"
#      labels = {
#        test = "yes"
#      }
#   }
#   spec {
#     client_id = "client"
#     client_secret = "value"

#     teams_to_logins {
#        organization = "gravitational"
#        team = "em"
#        logins = ["terraform"]
#     }
#   }
# }

# resource "teleport_trusted_cluster" "cluster" {
#   metadata {
#     name = "primary"
#     labels = {
#       test = "yes"
#     }
#   }

#   spec {
#     enabled = false
#     role_map {
#       remote = "test"
#       local = ["admin"]
#     }
#     proxy_address = "localhost:3080"
#     token = "salami"
#   }
# }

# resource "teleport_oidc_connector" "oidc" {
#   metadata {
#      name = "test"
#      labels = {
#        test = "yes"
#      }
#   }
#   spec {
#     client_id = "client"
#     client_secret = "value"

#     claims_to_roles {
#       claim = "test"
#       roles = ["terraform"]
#     }
#   }
# }

# resource "teleport_saml_connector" "saml" {
#   metadata {
#      name = "test"
#      labels = {
#        test = "yes"
#      }
#   }

#   spec {
#     issuer = "user"
#     assertion_consumer_service = "https://example.com"
#     entity_descriptor = <<EOT
# <md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="http://www.example.com/00000000000000000000">
#   <md:IDPSSODescriptor WantAuthnRequestsSigned="false" protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
#     <md:KeyDescriptor use="signing">
#       <ds:KeyInfo xmlns:ds="http://www.w3.org/2000/09/xmldsig#">
#         <ds:X509Data>
#           <ds:X509Certificate>Lu4bLZ57YSPClo5x1RHtXihqSdBfwqTU1tiPnL3i5QrHAXnyrmwscJ1VnutbfaTWCsPlICYQAVin
# vSAArSQU5WTjvZut9UeEenrYY72xDCLNe5vHimOEHFRvPeP626vx7/gkKSSL5F0Se+YYhLLCWcz8
# DYrQn41YZb72PBt5T0vIRS3FMZOYz55Ww8XbIWAwIKKmRfm00bPpMYPTD34ZCnVGTXSkHzHDCehu
# pQMug4IpWIcy45ffbi6sXoFD1ud8vG8H0RFhUk8MBFSCSsYHkrgz5cB8sbPLs0PocxN/nYIFJ2A1
# U68y2d3U/ClLfOb/kh4w3EcKvqtSwsMdLgxHjrDGtPgiAZDJhriZnpCQ0WvgBcAOYjRjsFncTRWH
# DqpTXsQzjkRa3A/KD3pA6bd5aYSF21nKAR7aVj7Aq0ogWEb4owZL5/W2lEnuwKSfGcnrz6GmJSaT
# 113wKahleH/VPb1KoaGJ81h5Om1DZI3ohYuxQYC/jwDhOlPXpdECkJe11gSTp34WQ1a93uSYkGo9
# MZ/7WI2LXpD6pjGtz5YSVKR1naj2pci5jwGi86KwL2MqXX288vguvGqcGZXUwi+383Ct99WLBNgo
# 9A6kIFvexILcscyeKthsoBGzu+MBipoGnSYuw+vlSa/0jIoluQqYpqYIg7ZBWoOjrKDDFdv01BtL
# nnVBFR43wCIm77obPQ5+103KYWcs42wpAxtX78HdlTav/D35D45GnGxM/fadpth65BSejgoPnd+z
# MXwMOv2W8B+fuolEcQGLrXw+mHtc2p3A7XKGhexY5A+FkSlAs3RMa0weizcylDlW2vj7ksdmZ/Ag
# AQ6EetT85DS6gV9wn3pBaWRhFU/OqFT/PezFcnxjiHVwfil+G9nhYhmjaspLqSLTkGPnyYabReZw
# ZtnSnKnWfwEr5GDqfYxHkBdZUtiofNhu/K/gs/aLTGoxWVac6F9y1xzXYnXPEPkmNsFfwn/H+LuL
# M01dKisWCfMPHCeBTxKSMB3IrixUym64cxlqkvk/rPXrUcktfvPhd/1I9jWIzQwPfbWyW9wpYzBm
# xYqZ1MocFyZhfh1UHOwaOiMlgAlOTDn6irtT1BW/a45nAkCl8jqgFKPSJ6kusj+HffSL6xDQJ0vA
# L5BGENThmToTm7euueLzYY0JDqhqo18wnha5MSCJtB3dcqKTeK+jiyF7FRHfZt/qJolXCufZyN48
# DQGrdrUjjolHvE8jmtgPkYuq9pdTciUnJIQN8vtQ/tOgk0Ui3n03FSM0YNARyaTZ0vgj+GLfGMc6
# VFKf6t/sSgFO8W4dgi2e0VwryOd8Etrq5NFul</ds:X509Certificate>
#         </ds:X509Data>
#       </ds:KeyInfo>
#     </md:KeyDescriptor>
#     <md:NameIDFormat>urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress</md:NameIDFormat>
#     <md:NameIDFormat>urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified</md:NameIDFormat>
#     <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="https://www.example.com/app/teleport/00000000000000000000"/>
#     <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://www.example.com/app/teleport/00000000000000000000"/>
#   </md:IDPSSODescriptor>
# </md:EntityDescriptor>
# EOT
#     entity_descriptor_url = "https://example.com"
#   }
# }