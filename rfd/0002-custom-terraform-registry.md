---
authors: Trent Clarke (trent@goteleport.com)
state: draft
---

# RFD 2 - Custom Terraform Registry
# Required Approvers
* Engineering: @r0mant && @klizhentas
* Product: (@klizhentas || @xinding33)

## What

A Terraform _registry_ is an index of available terraform _providers_. The
Terraform tool uses registries to locate, validate and install Terraform
providers for the various services required by terraform scripts. Hashicorp
(the Terraform vendor) publishes the protocol that Terraform uses to interact
with registries and so third parties (like us) may provide _custom_ Terraform
registries outside the main, Hashicorp-provided public registry.

This RFD investigates the use of a custom Terraform registry to distribute the
Teleport provider for Terraform.

## Why

We have two main goals that effect how we offer our Terraform provider to
customers

 1. We want to meet out customers where they are, e.g. if they're using
    Terraform, we want to supply them with a Terraform provider, and a method
    for installing it that is natural to the Terraform ecosystem. This is a
    good practice in general and, in the specific, is part of our Q1 focus on
    improving our Time-to_first-Value.

 2. We want to maintain close control of the distribution of our software,
    including supply chain management and monitoring of publishing, changes,
    downloads and host of other activities.

With these two goals in mind, the most practical solution for distributing
the Teleport Terraform provider appears to be hosting a custom Terraform
registry.

Note that the concerns in point 2 are being addressed in a more general scope
by [Cloud RFD 0004](https://github.com/gravitational/cloud/blob/master/rfd/0004-Release-Asset-Management.md),
and the solution proposed by this document will endeavour to be compatible
Cloud RFD 004 as far as possible.

### Why not use the public Hashicorp registry?

This conflicts with goal #2 (i.e. maintaining close control of the distribution
of our software).

Specifically, using the public Hashicorp registry requires that we grant Hashicorp
access to the Gravitational GitHub organisation via OAuth, and allow them to
manipulate the webhooks in our repositories.

### Will using a custom registry be onerous on our users?

No. Certainly less onerous than the current solution (involving several
manual download and installation steps).

The user would reference the custom registry directly in their terraform
scripts, and the Terraform tool would take care of the rest.

```terraform
terraform {
    required_providers {
        teleport = {
            source = "terraform.releases.teleport.dev/gravitational/teleport"
            version = "9.1.0"
        }
    }
}
```

For more info, see [the Terraform docs](https://www.terraform.io/cloud-docs/registry/using)
on using custom registries.

### What about name squatting on the Hashicorp registry?

What to do about people publishing homemade Teleport providers to the
Hashicorp registry is beyond the scope of this RFD.

It's worth noting, though, that _not_ being on the main Hashicorp registry _does_
make our official solution harder to find.

## Terminology

The terminology used in the Terraform documentation has changed over time and
is not always consistent. For the sake of this discussion, I will be using the
following definitions:

* **Provider**: The interface between Terraform and services and/or
                infrastructure to be managed.

* **State**: The Terraform representation of the current configuration of a
             resource. Used by Terraform to decide if a resource needs
             updating, and what changes to apply.

* **Provider Registry**: An index of available Terraform _providers_.

* **Terraform Cloud**: A hosted service run by Hashicorp that
  * hosts public and private _Provider Registries_
  * stores Terraform _State_, and
  * remotely executes Terraform scripts

* **Terraform Enterprise**: A privately-hosted version of _Terraform Cloud_.

* **Public Registry**: A publicly-accessible _registry_ that does not require
                       authentication to access.

* **Private Registry**: A _registry_ that requires authentication to access.

* **Custom Registry**: Any _registry_ not hosted by _Teleport Cloud_
                       or _Teleport Enterprise_. A custom implementation of
                       the Terraform Registry Protocol (see below).

The target of this RFD is, by the above definitions, a public, custom provider
registry.

### Terraform provider naming

Terraform providers have a two-part naming convention of `namespace/provider`,
where _namespace_ is the publishing organization and _provider_  is the the
actual name of the provider itself.

In this document I have used `gravitational` as the namespace and `teleport` as
the provider in order to disambiguate the two components of the provider name,
but there is no reason that we can't use `teleport/teleport` in the actual
deployment if that's the preferred branding.

## References

* [Cloud RFD 0004 - Release Asset Management](https://github.com/gravitational/cloud/blob/master/rfd/0004-Release-Asset-Management.md)
* [Cloud RFD 0017 - Artifact Storage Standards](https://github.com/gravitational/cloud/blob/master/rfd/0017-artifact-storage-standards.md)
* [Terraform Provider Registry Specification](https://www.terraform.io/internals/provider-registry-protocol)

## Details

### Terraform Registry Protocol requirements

For the full specification, see the [Provider Registry Specification](https://www.terraform.io/internals/provider-registry-protocol).

The major points of the registry protocol affecting this document are

1. A web service that exposes the registry index,
2. The binary packages referenced by the registry must be packaged in zipfiles,
   as opposed to the compressed tarballs our current build process produces, and
3. The checksum files must be signed with a GPG key

### Proposed Implementation

Although the Provider Registry Specification is written in terms of a web
service that will dynamically generate the appropriate responses, it is
_entirely possible_ to generate a registry index as a simple directory tree and
then statically serve those files to a client. This approach dramatically
simplifies the process of applying updates to the registry, and is therefore
the approach we will consider here.

#### Why can't we just use Houston?

The existing Houston distribution tool at [get.gravitational.com](https://get.gravitational.com/)
is not suitable for distributing the files indexed by the registry. Houston has
_very_ particular requirements about the naming of the files it serves (e.g.
all files are expected to be `*.tar.gz` compressed tarballs - or at least
_named_ as such) and, as it stands, will not serve the files we need it to.

We _could_ modify Houston to serve the required files, but as the service is
already slated for retirement, that does not seem to be a good use of time
& money.

### Overall architecture

In short:

* Separate _Staging_ and _Production_ registries.
  * _Staging_ will serve pre-release provider packages, and will be updated on
    every tag build,
  * _Production_ will serve released provider versions, and will be updated
    via promoting a specific tag build to production via Drone
* Each registry will be stored by an AWS S3 bucket and served by AWS CloudFront
* The storage bucket will be configured as per the Gold-class storage requirements
described in the [Cloud Artifact Storage Standards](https://github.com/gravitational/cloud/blob/master/rfd/0017-artifact-storage-standards.md)
RFD.

#### Public naming

All of the other Teleport distribution points ate subdomains of
`releases.teleport.dev`, so it makes sense for the production and staging
registries to live under this domain as well.

Staging: `terraform-staging.releases.teleport.dev`
Production: `terraform.releases.teleport.dev`

#### Bucket structure

The S3 bucket backing a registry requires 3 main structures:

1. the **_discovery file_**,
2. the **_registry_**, and
3. the **_object store_**

Each structure will live under a separate key prefix, which should allow
independent policies to be applied to each.

##### 1. The _discovery file_ (key `/.well-known/terraform.json`)

This implements the Terraform Remote Service Discovery Protocol. This is
essentially a static JSON file that redirects terraform where to the registry
service on a given host. (Note that it's also possible for this file to be
served from a completely different host.)

For example, this discovery protocol file:

```json
{
    "providers.v1": "https://terraform.releases.teleport.dev/registry/"
}
```

##### 2. The registry (key prefix `/registry`)

This is where the main registry metadata lives. The basic structure is described
in the [Provider Registry Specification](https://www.terraform.io/internals/provider-registry-protocol),
but in short there are two parts of the index for each `namespace/provider`
pair: the `versions` index, and the `download` metadata.

For our purposes, with a namespace of `gravitational` and a provider name of
`teleport`, the `versions` index file will be located at `/registry/gravitational/teleport/versions`

This is the only object in the registry that needs modification as part of
normal use.

The `download` metadata for each version/OS/architecture triple is stored
according to the rule

```shell
/registry/gravitational/teleport/:version/:os/:architecture
```

Once created, each `download` metadata does not need modification, unless
a provider object is re-published.

##### 3. The object store (key prefix `/store`)

The object store is where all the release package files are stored. Each
provider artefact, together with its associated hash & signatures files.

This section of the bucket needs only be write-once/read-many. No objects
in this section should be modified or deleted.

#### Build triggers

##### Staging Trigger

The staging registry will be updated on each _tag build_ (i.e. a build caused
by adding a git tag matching `terraform-provider-teleport-v*`).

**Note:** If there is a mismatch in the version number set in the plugins `Makefile` and
the tag value supplied by the release engineer, it's possible that later builds
will overwrite entries in the registry. (e.g. Makefile says `v1.2.3` and tags
are `terraform-provider-teleport-v1.2.3-rc.1`, `terraform-provider-teleport-v1.2.3-rc.2`,
etc). In this scenario, only the most recent build for a given `Makefile`-version
will be available via the registry.

##### Production Trigger

The production registry will be updated when a release engineer promotes a
given Drone build to either `production` or `production-terraform`.

The promotion process will perform the same actions as the staging script,
just with a different target bucket.

Ideally we would have been able to copy over the provider bundle from the
staging bucket, rather than repackage the original tarball. Unfortunately,
given the information available to us via Drone at promotion time, _and_ the
possibility of a provider in the staging bucket having been overwritten (see
[note above](staging-trigger)), it doesn't seem possible to guarantee that a
binary we're picking from the staging registry is the _exact same_ binary
originally constructed in the build to promote.

Going back to the original artifact tarball and repackaging it for Terraform
allows us to make that guarantee, so that is the process I've chosen.

#### Build inputs

##### Release Tarball

The Teleport provider release process currently produces a compressed tarball
by default, and this should still be considered the primary artefact product
of the _release_ build, at least as long as Houston is expected to serve
downloads of the Terraform provider.

The release tarball(s) will be injected into the build by being downloaded
from the staging bucket, based on the version tag supplied during the
promotion process.

##### Terraform Plugin API Version

In order to correctly index the providers, the registry need to know what
version of the Terraform Plugin API a given provider supports. This is not
something we can deduce from the provider itself, and must be "just known"
in advance.

This will be injected into the build via a value in the Drone yaml. There
is a danger that we will forget to modify this value should we upgrade the
version of the Teleport Plugin framework we use, and mislabel a release in
the registry.

##### Signing GPG Key

Packaging for the registry requires a GPG signing key, including an
identity.

Key is expected to be in PGP ASCII-armour format.

There is a discussion to be had about exact keys we should use for the staging
and production registries, but this is beyond the scope of this RFD.

Note that it's possible to change the keys that sign _new_ provider releases,
without having to go back and re-sign all of the previous releases, so changing
keys in future should be a straightforward operation.

#### Building the packages

As far as the registry protocol is concerned, the entire package for a given
provider consists of:

* The actual binary distribution zipfile
* A checksum file containing the SHA256 of the zipfile (formatted as per
 `sha256sum` tool)
* A signature file containing a binary, detached GPG signature for
  checksum file.

For each release tarball (one each for multiple release platforms - i.e.
`linux` and `darwin`):

1. deduce the version, OS and architecture of the release from the
    release tarball filename
2. repack the tarball into an appropriately formatted & named zipfile
3. generate the checksum file from the zipfile
4. generate the signature file by signing the checksum file

#### Updating the registry

To update the registry, we take the artefacts and data generated in the
previous step and

1. Download the existing `versions` index file (or create fresh, if
    not present)
2. Add entries for the new providers in to the index
3. Create the `download` entries for each file

I expect this process to be implemented as a small Go program (or a small suite
of them), rather than a shell script. Using small Go programs for build tasks
has shown itself to be a useful, flexible and above all _legible_ tool for
scripting build tasks in the mainline Teleport repository, and we should
continue that pattern in `teleport-plugins`.

#### Final publishing

Upload the repackaged `zip` files, the updated `versions` index and the new
`download` files to the S3 bucket.

#### Synchronization Hazard

The Download-Update-Publish cycle for updating the registry index is vulnerable
to corruption if multiple independent updates occur simultaneously.

Unfortunately, AWS S3 does not provide any built-in methods for preventing this
sort of issue.

It also apparently lacks the primitives that would allow us to layer one over S3:

* There is no way to push an object to a bucket only if an object with the same
  key does not already exist, which means we can't really implement a race free
  lockfile-like protocol
* There are no compare-and-swap style put primitives
* While versioning _is_ supported, there is no way to say "only overwrite object
  _X_ if its current version is _Y_".

Our initial approach may have to be relying on the build serialisation tools
provided by Drone.

#### Build Security issues

This process will require that the `teleport-plugins` promotion process have
access to new secrets & resources:

##### Staging Resources

* Staging Registry Bucket
* Staging Registry CloudFront
* Staging Registry logs bucket
* Staging IAM User with read/write access to Staging Registry Bucket
  * Modify permission required on `versions` file
* Staging signing key

##### Production Resources

* Production Registry Bucket
* Production Registry CloudFront
* Production Registry logs bucket
* Production IAM User with read/write access to Production Registry Bucket
  * Modify permission required on `versions` file
* Production signing key

_**Note:**_ We currently expect to use the RPM signing key as the Terraform
production signing key as it already exists and is already used for a similar
purpose.

That said, there is no technical reason why it _must_ be the RPM signing
key. As long as the private key used to sign the package agrees with the a
public key exposed via the registry, the signature should be deemed valid
by Terraform.

_**Also note**_ that we can rotate the key used for signing _new_ additions to
the registry without having to re-sign the entire registry, meaning that we
can get started using the existing RPM signing key and cheaply rotate it when
a new key management structure is in place.

### Testing

The system will be tested by promoting to a "staging" environment. The
staging environment will be selected during the Drone promotion process
and will use a dummy bucket and signing key.

## Apendices

### Terraform Plugin API version

Hashicorp versions the API that Terraform uses to communicate with providers,
and (very) occasionally it increases the version number and breaks
compatibility with older versions of Terraform.

In order to properly index the terraform plugins, a provider registry needs
to know the versions of the Terraform Plugin API supports. I haven't found
a good way to extract the Plugin API version directly from the provider binary,
so we are relying on the Drone trigger to pass in the correct versions.

I've looked into how Terraform itself does this. The mechanism is baked into a
bunch of `internal` packages used by the terraform command line tool. We
certainly _could_ extract the code we need to interrogate the binary for the
API version, but it would be an exceedingly fragile solution.

Manually updating the API version in the Drone file is probably the most
pragmatic way of dealing with API changes, given how infrequently this
number changes.
