# Terraform Provider Plugin

## Usage

Please, refer to [GETTING_STARTED guide](GETTING_STARTED.md) and [official documentation](https://goteleport.com/docs/setup/guides/terraform-provider/).

## Development

1. Install [`protobuf`](https://grpc.io/docs/protoc-installation/).
2. Install [`protoc-gen-terraform`](https://github.com/gravitational/protoc-gen-terraform) v1.0.0+.

    ```go install github.com/gravitational/protoc-gen-terraform@a63aa54956b6bbbcdac039d2f54261bae12d19e8```

    _NOTE_: Once PR is merged, we'll replace SHA with v1.0.0

3. Install [`Terraform`](https://learn.hashicorp.com/tutorials/terraform/install-cli) v1.1.0+. Alternatively, you can use [`tfenv`](https://github.com/tfutils/tfenv). Please note that on Mac M1 you need to specify `TFENV_ARCH` (ex: `TFENV_ARCH=arm64 tfenv install 1.1.6`).

4. Clone the plugin:

    ```bash
    git clone git@github.com:gravitational/teleport-plugins --branch chore/terraform-refactoring
    ```

    _NOTE_: Once PR is merged, we'll remove --branch

5. Build and install the plugin:

    ```bash
    cd teleport-plugins/terraform
    make install
    ```

6. Run tests:

    ```bash
    make test
    ```

# Updating the provider

Run:

```
make gen-tfschema
```

This will generate `types_tfschema.go` from a current API `.proto` file, and regenerate the provider code.

# Running the examples

WIP

<!-- 1. Run `cp example/vars.tfvars.example example/vars.tfvars`.
2. Replace `github_secret` and `saml_entity_descriptor` with the actual values (where `github_secret` could be random, and `saml_entity_descriptor` should be a real entity descriptor taken from OKTA). -->
