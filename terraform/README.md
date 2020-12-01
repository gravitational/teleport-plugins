# Terraform Provider Plugin

## Current Status

This is an early proof of concept, expect a lot of things to not work.

### Building the provider

The `terraform` directory has it's own go module defined, but some dependencies
versions are pinned to the same versions Teleport itself uses.

- Separate go module makes sense because we'll likely move the provider to a
  separate repository in the future, and will build it independently of the
  other Teleport Plugins.
- Pinning dependency versions to Teleport's deps is required becuase for now,
  the provider depends on the whole Teleport, and to build it, we need
  compatible deps.

To build the provider binary: `go build -o build/terraform-provider-teleport`

### Testing the provider

One way to test the provider without having your own terraform project is by
setting up your Teleport certificates and address in `main.tf` and then
`terraform plan`.

## Project Description

Teleport 5.1 will open source role based access control.

To give it a boost in the community, implement a Terraform provider for teleport
roles, connectors and other resources.

Notes:

Use native client in teleport/lib/auth/Client

See the similar provider for Gravity Enterprise for resource specifications for
roles and trusted clusters. See the example (obsolete) code here.

Authentication of the provider should be done via certificate and role issued
using `tctl auth sign` if provider is accessing a cluster remotely, or
automatically using built-in certificate if the provider is running alongside
teleport auth server.

Add support for the following resources:

- OIDC/SAML/Github Connectors for Enterprise
- Github connectors for SSO
- Trusted clusters
- Roles
- Users
- Tokens

Support Terraform >= v0.13.

Deleting a resource from terraform should delete a role from teleport. Updating
or creating a role should update or create a role in teleport using golang API.

Test the provider with OSS, Enterprise and Cloud versions.
