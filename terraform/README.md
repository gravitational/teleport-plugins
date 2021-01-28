# Terraform Provider Plugin

## Current Status

Work in progress.

Build requirements:

- Install
  [protoc-gen-tfschema](https://github.com/nategadzhi/protoc-gen-tfschema)
- `gravitational/teleport` source available

```shell

go install github.com/nategadzhi/protoc-gen-tfschema

# Substitute your path to teleport source
protoc --proto_path=. \
  --proto_path=/Users/xnutsive/go/src/github.com/gravitational/teleport/vendor/github.com/gogo/protobuf \
  --proto_path=/Users/xnutsive/go/src/github.com/gravitational/teleport/lib/services \
  --proto_path=/Users/xnutsive/go/src \
  types.proto \
  --tfschema_out=./tfschema/ \
  --go_out=./tfschema/ \
  --tfschema_opt="types=UserV2"

```

### Building the provider

**Dependencies**

The `terraform` directory has it's own go module defined, but some dependencies
versions are pinned to the same versions Teleport itself uses.

- Separate go module makes sense because we'll likely move the provider to a
  separate repository in the future, and will build it independently of the
  other Teleport Plugins.
- Pinning dependency versions to Teleport's deps is required becuase for now,
  the provider depends on the whole Teleport, and to build it, we need
  compatible deps.

**Dev install**

To build and install the provider in development, set the architecture in
`Makefile` (should be `linux_amd64` or `darwin_amd64`), then `make build`.

Teleport provider is currently an "in house" provider, meaning it's not
distributed via Hasicorp's Terraform Registry.

`terraform` directory contains `main.tf` — a minimal Terraform demo project that
uses Teleport Provider to provision Teleport Users.

**To use the Provider, you'll need to provision a Teleport cluster, and set the
Auth server address and certificate paths in `main.tf`**

**Docker flow**

`docker` dir in this repo has a set of tools to run Teleport and it's plugins
locally. You can use that to build an image with Teleport Enterprise and manage
certificates:

```shell
# cd to the docker dir
cd ../docker

# make sure you have teleport-ent image built.
# this requires teleport:latest image, that can be built by cd gravitational/teleport/docker && make build
make teleport-ent

# bootstrap the cluster
# if this fails and says it can't connect to teleport, run this again. ;)
make config

# back to terraform dir
cd ../terraform

# build telepor-terraform:latest, the terraform docker VM we're going to use.
# It's just a buildbox with terraform-cli installed.
make docker
```

At this point, you'll have a Teleport Enterprise cluster with a user Foo and
their cerfiticates ready to go.

````shell
cd ../docker

# run teleport on localhost:3080 (web) and localhost:3025 (auth)
docker-compose up teleport
---

Now, you can user the following Terraform provider config in `docker-compose run terraform /bin/bash`.

```HCL
terraform {
  required_providers {
    teleport = {
      versions = ["0.0.1"]
      source = "gravitational.com/teleport/teleport"
    }
  }
}

# Assuming you're running in docker-compose run terraform
provider "teleport" {
    addr = "teleport.cluster.local:3025"
    cert_path = "/mnt/shared/certs/access-plugin/auth.crt"
    key_path = "/mnt/shared/certs/access-plugin/auth.key"
    root_ca_path = "/mnt/shared/certs/access-plugin/auth.cas"
}

````

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
