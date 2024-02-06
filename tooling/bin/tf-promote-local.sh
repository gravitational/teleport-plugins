#!/bin/bash

if [ "$#" -ne 6 ]; then
  echo "Usage: $0 <artifact version (i.e. v14.3.0)> <artifact bucket (i.e. tp-146628656107-us-west-2-build-artifacts-prod)> <artifact bucket AWS profile name>" \
    "<registry bucket (i.e. tp-146628656107-us-west-2-terraform-registry-store-prod)> <release bucket AWS profile name> <registry URL (i.e. https://terraform.releases.teleport.dev/)>"
  exit 1
fi

if [ -z "$SIGNING_KEY" ]; then
  echo "The 'SIGNING_KEY' variable must be set with the GPG signing key in ASCII armor format"
  exit 1
fi

set -eu

ARTIFACT_TAG="$1"
ARTIFACT_BUCKET="$2"
ARTIFACT_BUCKET_PROFILE="$3"
ARTIFACT_BUCKET_PATH="s3://$ARTIFACT_BUCKET/teleport-plugins/tag/terraform-provider-teleport-$ARTIFACT_TAG/"
ARTIFACT_DIRECTORY=$(mktemp -d -t "terraform-promotion-artifacts")

REGISTRY_BUCKET="$4"
REGISTRY_BUCKET_PROFILE="$5"
REGISTRY_BUCKET_PATH="s3://$REGISTRY_BUCKET/"
REGISTRY_DIRECTORY=$(mktemp -d -t "terraform-provider-registry")
REGISTRY_URL="$6"

echo "Downloading artifacts to $ARTIFACT_DIRECTORY from artifact storage bucket path $ARTIFACT_BUCKET_PATH with via $ARTIFACT_BUCKET_PROFILE profile"
export AWS_PROFILE="$ARTIFACT_BUCKET_PROFILE"
aws sts get-caller-identity
aws s3 sync "$ARTIFACT_BUCKET_PATH" "$ARTIFACT_DIRECTORY"

echo "Downloading a local copy of the Terraform registry to $REGISTRY_DIRECTORY from bucket path $REGISTRY_BUCKET_PATH via $REGISTRY_BUCKET_PROFILE profile"
export AWS_PROFILE="$REGISTRY_BUCKET_PROFILE"
aws sts get-caller-identity
aws s3 sync "$REGISTRY_BUCKET_PATH" "$REGISTRY_DIRECTORY"

go run -C tooling ./cmd/promote-terraform         \
  --tag "$ARTIFACT_TAG"                           \
  --artifact-directory-path "$ARTIFACT_DIRECTORY" \
  --registry-directory-path "$REGISTRY_DIRECTORY" \
  --protocol 6                                     \
  --registry-url "$REGISTRY_URL"                  \
  --namespace gravitational                       \
  --name teleport

echo "Syncing the local copy of the Terraform registry from $REGISTRY_DIRECTORY to bucket path $REGISTRY_BUCKET_PATH"
aws s3 sync --dryrun "$REGISTRY_DIRECTORY" "$REGISTRY_BUCKET_PATH"
