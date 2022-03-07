---
authors: Trent Clarke (trent@goteleport.com)
state: draft
---

# RFD 2 - Custom Terraform Registry

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
    for installing it that is natural to the Terraform ecosystem.

 2. We want to maintain close control of the distribution of our software.

With these two goals in mind, the most practical solution for distributing
the Teleport Terraform provider appears to be hosting a custom Terraform
registry.

### Why not use the public Hashicorp registry?

This conflicts with goal #2 (i.e. maintaining close control of the
distribution of our software).

### Will using a custom registry be onerous on our users?

No. Certainly less onerous than the current solution (involving several
manual download and installation steps).

The user would reference the custom registry directly in their terraform
scripts, and the Terraform tool would take care of the rest.

```terraform
terraform {
    required_providers {
        teleport = {
            source = "http://goteleport.com/api/teleport/"
            version = "9.1.0"
        }
    }
}
```

For more info, see [the Terraform docs](https://www.terraform.io/cloud-docs/registry/using)
on using custom registries.

## Details

### Terraform Registry Protocol requirements

For the full specification, see the [Provider Registry Specification](https://www.terraform.io/internals/provider-registry-protocol).

The major points of the registry protocol affecting this document are

1. A web service that exposes the registry index,
2. The binary packages referenced by the registry must be packaged in zipfiles,
   as opposed to the compressed tarballs our current build process produces, and
3. The checksum files must be signed with a GPG key

### Serving the registry and packages

#### Registry Hosting

The prototype registry (under development) is currently set up to be served as
part of the main Teleport website (i.e. `http://goteleport.com/api/teleport/teleport/`).

This implies that the public-facing registry code will live in `gravitational/next`.

There appears to be no requirement for the files indexed by the registry to be
served from the same host as the registry, so the actual package files will be
hosted externally.

#### Package Hosting

The distribution files (i.e. the `zip`, checksum and signature files) indexed
by the registry will be stored in an AWS S3 bucket, and exposed to the Internet
via a CloudFront CDN service.

The storage bucket will be configured as per the Gold-class storage requirements
described in the [Cloud Artifact Storage Standards](https://github.com/gravitational/cloud/blob/master/rfd/0017-artifact-storage-standards.md)
RFD.

#### Why can't we just use Houston?

The existing Houston distribution tool at [get.gravitational.com](https://get.gravitational.com/)
is not suitable for distributing the files indexed by the registry. Houston has
_very_ particular requirements about the naming of the files it serves (e.g.
all files are expected to be `*.tar.gz` compressed tarballs - or at least
_named_ as such) and, as it stands, will not serve the files we need it to.

We _could_ modify Houston to serve the required files, but as the service is
already slated for retirement, that does not seem to be a good use of time
& money.

### Building provider packages

As far as the registry protocol is concerned, the entire package for a given
provider consists of:

* The actual binary distribution zipfile
* A checksum file containing the SHA256 of the zipfile (formatted as per
 `sha256sum` tool)
* A signature file containing a binary, detached GPG signature for
  checksum file.

The Teleport provider release process currently produces a compressed tarball
by default, and this should still be considered the primary artefact produced
by the build, at least as long as Houston is expected to serve downloads of
the Terraform provider.

By analogy with the main Teleport release system, the registry-compatible
provider package will be created during release promotion, with a build task
that will:

1. For each release tarball (one each for multiple release platforms):
    1. repack the tarball into an appropriately formatted & named zipfile
    2. generate the checksum file from the zipfile
    3. generate the signature file by signing the checksum file
2. Upload all generated files to the AWS bucket that backs the registry
3. Update the registry index to include the newly released providers (see
   _Populating the registry_, below)

I expect this to be implemented as a small Go program, rather than a shell
script. Using small Go programs for build tasks has shown itself to be a
useful, flexible and above all _legible_ tool for scripting build tasks in
the mainline Teleport repository, and we should continue that pattern in
`teleport-plugins`.

#### Build Security issues

This process will require that the `teleport-plugins` promotion process will
need new secrets:

1. Read access to a signing key, to sign the package (see note below)
2. Write access to the AWS bucket storing the signed packages for distribution

_**Note:**_ We currently expect to use the RPM signing key as the Terraform
signing key as it already exists and is already used for a similar purpose.
That said, but there is no technical reason why it _must_ be the RPM signing
key. As long as the private key used to sign the package agrees with the a
public key exposed via the registry, the signature should be deemed valid
by Terraform.

### Populating the registry

Populating the registry is a two-part problem:

First, we must create a new, back-filled registry containing all of the
currently available Teleport terraform providers.

This appears to be pretty straightforward, as I already have a proof-of-
concept program that scans the release plugins AWS bucket and repacks all
of the available releases, generating a TypeScript dictionary that can be
embedded directly in the front-end registry code.

Secondly, we need a mechanism to _maintain_ the registry as new releases
are produced.

_This_ is the major open question in this RFD - _how will our promotion
process communicate to our custom registry that a new release exists?_

#### Options

1. Fully manual process
2. Promotion process generates an index file that can be used by the
   registry in `gravitational/next`, either by
    1. Scraping by `next` at build time to generate static data in the registry,
    2. The registry in `next` fetching and parsing the index at runtime
    3. Some other mechanism?
3. Something else entirely?
