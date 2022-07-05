# Teleport Plugins Release Process

The Teleport plugins release process is controlled by Drone, and is described in detail by [RFD003](./rfd0003-teleport-plugins-versioning.md).

## Authenticating to Amazon ECR
Plugin maintainers can push to both the staging and public Amazon Elastic Container Registries by assuming the `release-engineer-plugin-admin` role in AWS. To assume this administrative role the engineer must first login to AWS using `Plugin-Release-Engineers` permission set. This permission set can be found under the `teleport-prod` AWS Account. To request access to this account please contact the IT team. 

Assume the `release-engineer-plugin-admin` on the CLI with:
```console
$ aws sts assume-role --role-arn arn:aws:iam::146628656107:role/release-engineer-plugin-admin --role-session-name AWSCLI-Session
``` 
and export the credentials to your environment. 

Once authenticated, the plugin maintainer can authenticate to the staging and public ECR's using the following commands, respectively. 

```console
$ aws ecr get-login-password --region us-west-2 | docker login -u="AWS" --password-stdin 146628656107.dkr.ecr.us-west-2.amazonaws.com
```
and
```console 
$ aws ecr-public get-login-password --region us-east-1 | docker login -u="AWS" --password-stdin public.ecr.aws
```

Whenever possible, the plugin maintainers should prefer to push to these registries by tagging a new version and promoting it in drone. However, in case of emergency the above method may be used to push an image. 

### Pull an image in staging
Teleport engineers can gain read-only access to the internal plugin images by authenticating to AWS using the `Teleport-Team-Prod-ReadOnly` permission set found under the `teleport-prod` AWS Account. 

Afterwards, authenticate to the registry with:

```console
$ aws ecr get-login-password --region us-west-2 | docker login -u="AWS" --password-stdin 146628656107.dkr.ecr.us-west-2.amazonaws.com
```

and pull an image with:
```console
$ docker pull 146628656107.dkr.ecr.us-west-2.amazonaws.com/gravitational/teleport-plugin-email:9.3.7
```

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