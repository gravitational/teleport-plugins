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
    for installing it that is natural to the Terraform ecosystem. This is a
    good practice in general and, in the specific, is part of our Q1 focus on
    improving out Time-to_first-Value.

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

* **Provider**: The interface between Terraform and an arbitrary resource to
                be managed.

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

#### Overall architecture

The pre-built registry will be stored in an AWS S3 bucket, and exposed
to the Internet via a CloudFront CDN service.

The storage bucket will be configured as per the Gold-class storage requirements
described in the [Cloud Artifact Storage Standards](https://github.com/gravitational/cloud/blob/master/rfd/0017-artifact-storage-standards.md)
RFD.

#### Public naming

I have assumed in this document that the public-facing address for the
terraform registry host will follow the naming convention for other Teleport
distribution points, and be `terraform.releases.teleport.dev`.

#### Bucket structure

The S3 bucket backing the registry requires 3 main structures:

1. the **_discovery file_**,
2. the **_registry_**, and
3. the **_object store_**

Each structure will live under a separate key prefix, which should allow 
independent policies to be applied to each.

##### 1. The _discovery file_ (key `/.well-known/terraform.json`)

This implements the Terraform Remote Service Discovery Protocol. This is
essentially a static JSON file that redirects terraform where to the registry
service on a given host.

For example, this discovery protocol file:

```json
{
    "providers.v1": "/registry/"
}
```

_**NOTE:**_ The address in the discovery protocol JSON need not be a relative
path - it could _in theory_ point to a totally different host. It _should_ be
possible to have the discovery protocol served by `goteleport.com` that would
direct terraform to `terraform.releases.teleport.dev` for the actual registry,
if that's the preferred branding.

Note that I have not _personally_ tested this behaviour.

The practical change this would make would be that the provider source would
change from (in the example above):

```terraform
source = "terraform.releases.teleport.dev/gravitational/teleport"
```

to

```terraform
source = "goteleport.com/gravitational/teleport"
```

##### 2. The registry (key prefix `/registry`)

This is where the main registry metadata lives. The basic structure is described
in the [Provider Registry Specification](https://www.terraform.io/internals/provider-registry-protocol),
but in short there are two parts of the index for each `namespace/provider`
pair: the `versions` index, and the `download` metadata.

For our purposes, with a namespace of `gravitational` and a provider name of
`teleport`, the `versions` index file will be located at `/registry/gravitational/teleport/versions`

This is the only object in the repository that need modification as part of
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

This section of the bucket should be write-only. No objects in this section
should be modifiable or delete-able.

#### Build trigger and inputs

By analogy with the main Teleport release system, the Terraform registry
packaging and publishing will be happen during release _promotion_ task on
Drone.

##### Release Tarball

The Teleport provider release process currently produces a compressed tarball
by default, and this should still be considered the primary artefact product
of the _release_ build, at least as long as Houston is expected to serve
downloads of the Terraform provider.

The release tarball(s) will be injected ibto the build by being downloaded
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

Any suggestions on an interlock for this value greatly appreciated.

##### Signing GPG Key

Packaging for the registry requires a GPG signing key, including an
identity. The Identity is displayed to the user when the provider is
installed, so it should be an "official" teleport key rather than a
self-signed one

For example (from my PoC):

```shell
Initializing provider plugins...
- Finding terraform.clarke.mobi/gravitational/teleport versions matching "~> 8.3"...
- Installing terraform.clarke.mobi/gravitational/teleport v8.3.4...
- Installed terraform.clarke.mobi/gravitational/teleport v8.3.4 (self-signed, key ID 7DA6C64E1701F9E4)
```

My proof-of-concept cose expects the signing key in ASCII-armor format, but I
am sure others are acceptable as well.

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

I expect this process to be implemented a sa small Go program (or a small suite
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

This process will require that the `teleport-plugins` promotion process will
need new secrets:

1. Read access to a signing key, to sign the package (see note below)
2. Write access to the AWS bucket storing the signed packages for distribution

_**Note:**_ We currently expect to use the RPM signing key as the Terraform
signing key as it already exists and is already used for a similar purpose.
That said, there is no technical reason why it _must_ be the RPM signing
key. As long as the private key used to sign the package agrees with the a
public key exposed via the registry, the signature should be deemed valid
by Terraform.

### Testing

The system will be tested by promoting to a "staging" environment. The
staging environment will be selected during the Drone promotion process
and will use a dummy bucket and signing key.
