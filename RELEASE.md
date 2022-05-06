# Teleport Plugins Release Process

The Teleport plugins release process is controlled by Drone, and is described in detail by [RFD003](./rfd0003-teleport-plugins-versioning.md).

## Recovery & Troubleshooting

### Terraform Registry

You can manually invoke the terraform registry promotion to fix a failed
provider release. In order to do this you will need:

* this repository
* access to the `sops`-encrypted Drone secrets file for the teleport-plugins
  build
* access to the key to decrypt the secrets file
* the git tag of the provider to publish (e.g. `terraform-provider-teleport-v9.1.3`)
  
#### Step 1: Build the secret injection shim

See `tooling/with-secrets/main.go` for details about why this shim is necessary.

```sh
$ (cd tooling; go build -o .. ./cmd/with-secrets)
```

#### Step 2: Publish the provider

There are scripts to download, repackage and push the provider archive to the
staging and production terraform registries under `./tooling/bin`. They take all
the same actions as the drone promotion pipeline, but are much easier to
troubleshoot. They can be run like so:

```sh
$ sops -d $plugins_secrets | ./with-secrets ./tooling/bin/release $terraform_provider_version
```