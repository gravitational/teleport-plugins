# Terraform Provider Plugin

## Current Status

This is a sandbox / proof of concept at this point.

To compile the provider: `go build -o teleport-provider -mod=mod`. go.mod
screwed up vendoring and I don't want to yak shave it just yet.

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
