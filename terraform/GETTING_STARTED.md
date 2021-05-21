# Getting started with clean Terraform setup

We assume you have docker and Teleport installed on your system.

# Install Terraform

Check [Terraform official docs](https://learn.hashicorp.com/tutorials/terraform/install-cli) for installation instructions. Terraform 0.12 or higher is required.

# Install provider

_NOTE: This won't be necessary once we publish provider in Terraform registry. Repo URL will also change._

```
git clone --depth 1 git@github.com:gravitational/teleport-plugins.git
cd teleport-plugins/terraform/build.assets
make install
```

It will build and put latest provider binary to your `~/terraform.d` folder. 

# Create Terraform directory

```
mkdir ~/terraform-cluster && cd ~/terraform-cluster
```

# Create Terraform user in Teleport

Put the following content into terraform.yaml:

```
// terraform.yaml
kind: role
metadata:
  name: terraform
spec:
  allow:
    rules:
      - resources: ['user', 'role', 'token', 'trusted_cluster', 'github', 'oidc', 'saml']
        verbs: ['list','create','read','update','delete']
version: v3
---
kind: user
metadata:
  name: terraform
spec:
  roles: ['terraform']
version: v2
```

Run:

```
tctl create terraform.yaml
tctl auth sign --format=file --user=terraform --out=terraform-identity --ttl=10h
```

Note: Teleport cloud users may want to use [impersonation](https://goteleport.com/docs/access-controls/guides/impersonation/) for this step.

# Create Terraform configuration

Copy this to main.tf:

```
terraform {
  required_providers {
    teleport = {
      version = "0.0.1"
      source  = "gravitational.com/teleport/teleport"
    }
  }
}

provider "teleport" {
  # Update addr to point to Teleport Auth/Proxy
  # e.g. addr = "example.teleport.sh:3025"
  addr               = "localhost:3025"
  identity_file_path = "terraform-identity"
}

resource "teleport_user" "example" {
  metadata {
    name        = "example"
    description = "example"

    labels = {
      test      = "true"
    }
  }

  spec {
    roles = ["admin"]
  }
}

resource "teleport_role" "example" {
  metadata {
    name        = "example"
    description = "Example Teleport Role"
    expires     = "2022-10-12T07:20:50.52Z"
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
```

Check the [documentation](TODO: generate documentation and add the link here) for all available schema definitions.

# Apply the configuration

```
terraform init
terraform apply
```
