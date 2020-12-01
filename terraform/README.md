# Terraform Provider Plugin

## Current Status

This is an early proof of concept, expect a lot of things to not work.

**Users**

* [x] Create users
* [x] Update users
* [x] Delete users

**Roles**

* [ ] Create roles
* [ ] Update roles
* [ ] Delete roles

**Testing framework**


**Validation framework**


## Known issues

User traits are a string where trait values are separated by space, since
Terraform Schema Map doesn't provide an out of the box way to do a map of string
to a list. One way to improve this is to define a Trait as a separate piece of
data that can be nested in a resource, i.e:

```
"teleport_user" "tf_player_one" {
  name = "nate"
  roles = ["foo"]

  trait {
    name = "logins"
    values = [
      "root"
    ]
  }
}
```

### Building the provider

**Dependencies**

The `terraform` directory has it's own go module defined, but some dependencies
versions are pinned to the same versions Teleport itself uses.

* Separate go module makes sense because we'll likely move the provider to a
  separate repository in the future, and will build it independently of the
  other Teleport Plugins.
* Pinning dependency versions to Teleport's deps is required becuase for now,
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

* OIDC/SAML/Github Connectors for Enterprise
* Github connectors for SSO
* Trusted clusters
* Roles
* Users
* Tokens

Support Terraform >= v0.13.

Deleting a resource from terraform should delete a role from teleport. Updating
or creating a role should update or create a role in teleport using golang API.

Test the provider with OSS, Enterprise and Cloud versions.
