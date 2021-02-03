# Terraform Provider Plugin

## Current Status

Work in progress.

Build requirements:

- Install
  [protoc-gen-tfschema](https://github.com/nategadzhi/protoc-gen-tfschema)
- `gravitational/teleport` source to generate schemas

```shell

go install github.com/nategadzhi/protoc-gen-tfschema

# Substitute your path to teleport source
protoc --proto_path=. \
  --proto_path=/Users/xnutsive/go/src/github.com/gravitational/teleport/vendor/github.com/gogo/protobuf \
  --proto_path=/Users/xnutsive/go/src/github.com/gravitational/teleport/lib/services \
  --proto_path=/Users/xnutsive/go/src \
  types.proto \
  --tfschema_out=./tfschema/
```

### Building the provider

`make build` will do the following:
- Remove terraform state and locks from `./example` (it's used for dev purposes for now)
- Remove terraform provider from your `~/.terraform` if it's already there.
- Clean the `build` directory
- Build the provider
- Install it into terraform's preferred directory


To run the provider:
1. Make sure you setup teleport config in `./example/main.tf`. **You can user `make -C teleport-plugins/docker foo-certs` to export certificates to `teleport-plugins/docker/tmp`**
2. Make sure Teleport instance is up and running. You can use the Docker flow (teleport-plugins/docker).
2. `make reapply` will rebuild the provider, re-init terraform in `./example` and run `terraform apply -auto-approve`.

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

```

```
