---
authors: Brian Joerger (bjoerger@goteleport.com)
state: draft
---

# RFD 3 - Teleport Plugins releases

## What

Release strategy for Teleport plugins.

## Why

With the release of the Teleport API as a [go module](https://pkg.go.dev/github.com/gravitational/teleport)
and the addition of new plugins (Terraform) which must be updated for every new 
version, it's time to layout a proper release process.

## Details

Teleport plugins are released in line with Teleport releases. This leads to the
plugins following the same [versioning scheme](https://github.com/gravitational/teleport/blob/master/rfd/0012-teleport-versioning.md#rfd-12---teleport-versioning)
as Teleport, and more importantly, the same [compatibility guidelines](https://github.com/gravitational/teleport/blob/master/rfd/0012-teleport-versioning.md#compatibility).

These releases are more often than not just vanity releases, with no functional
changes in them. When there are functional changes in a plugin, they must follow 
the same compatibility guidelines above.

### Release process

The full release process is contained in `make release-version VERSION=vX.Y.Z`.
This will update the version files of each plugin, update the `github/gravitational/teleport/api`
dependency, regenerated the Terraform Schema, and update the git tags.

Some basic manual changes may need to be made to handle significant changes
made to the Teleport API, as explained below.

#### Teleport API dependency

The `github/gravitational/teleport/api` dependency should be updated with every
Teleport Plugins release to ensure compatibility with its corresponding Teleport
version. This can be done with `go get github.com/gravitational/teleport/api@vX.Y.Z`,
which can be added to the `make update-version` target.

However, this may lead to errors if significant changes have been made to the API,
and will require some manual work to resolve. 

To avoid delaying releases, a PR should be made as soon as `teleport@vX.Y.Z-beta.1`
is released to update the Plugins version to `vX.Y.Z-beta.1`. Any issues can then 
be resolved well before `teleport@vX.Y.Z` is released, and updated afterwards.

Alternatively, if we shift towards using release branches (detailed below), then 
the beta PR canbe merged into `branch/vX` and then another PR can be made once
`teleport@vX.Y.Z-beta.1` is released.

#### Terraform Provider

The Terraform Provider uses `teleport/api/types.proto` directly to generate a
Terraform Schema for Teleport resources. Therefore every time the `teleport/api` 
package is upgraded, we must regenerate the Terraform Schema.

This is handled in two steps:
    - Update `terraform/gen_teleport.yaml`
        - If any resources have been upgraded (e.g. `RoleV4` -> `RoleV5`), those
        changes must be reflected here. Any upgrades needed should be easy to spot
        as they can be seen in the linting errors from updating the API dependency.
        - Other sections may need to be updated, such as `sensitive fields`.
    - Run `make -C terraform gen-schema`

In the future, we can consider using [buf](https://docs.buf.build/introduction)
or something similar to handle `.proto` dependencies more carefully, and avoid
the need for unguided manual fixes.

#### Integration Tests

The repo's integration tests directly download versions of `teleport`, `tsh`, and `tctl`
to handle integration testing. The version downloaded should be updated at least every
major release or else the tests will fail or fail to fail when something is broken.

The version referenced by `TELEPORT_GET_VERSION` in the drone and cloudbuild files should
be updated, and the version should be added to `lib/integration/testing/download.go` with
the correct sha256 values.

### Additional Concerns

#### Release branches

The teleport-plugins repository only makes releases from `master`. This means
that new features will always go into the next release, and change and fixes 
can not be easily back-ported. 

This has not been an issue thus-far, but in the future we can consider adopting
the same [release branches](https://github.com/gravitational/teleport/blob/master/rfd/0012-teleport-versioning.md#git-branches)
strategy as Teleport.