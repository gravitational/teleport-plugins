# Terraform Provider Plugin

## Usage

Please, refer to [GETTING_STARTED guide](GETTING_STARTED.md) and [official documentation](https://goteleport.com/docs/setup/guides/terraform-provider/).

## Development

1. Install [`protobuf`](https://grpc.io/docs/protoc-installation/).
2. Install [`protoc-gen-terraform`](https://github.com/gravitational/protoc-gen-terraform) v1.0.0+.

    ```go install github.com/gravitational/protoc-gen-terraform@ed458d13eb0fc66b5bebbd3d6dc6897b8cd751ef```

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

# Regenerating the schema

Run:

```
make gen-tfschema
```

# Usage

See `example/*.tf` for available configuration options. `make apply` to do an initial application of this configuration to your Terraform cluster.

---

4. If you desire to use an example for testing:

```bash
cp example/vars.tfvars.example example/vars.tfvars
```

Edit `vars.tfvars` and set path to certificate files which were generated in the previous step.
