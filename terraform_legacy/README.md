# Terraform Provider Plugin

# Installation

1. Clone the plugin:

```bash
git clone git@github.com:gravitational/teleport-plugins
```

2. Install the plugin to Teleport:

```bash
cd teleport-plugins/terraform
make install
```

3. Configure teleport:

```bash
tctl create example/teleport.yaml
tctl auth sign --format=file --user=terraform --out=terraform-identity --ttl=10h
```

Move generated keys to the desired location.

4. If you desire to use an example for testing:

```bash
cp example/vars.tfvars.example example/vars.tfvars
```

Edit `vars.tfvars` and set path to certificate files which were generated in the previous step.

# Regenerating the schema

```
go install github.com/gravitational/protoc-gen-terraform
make gen-schema
```

Please note that `ProvisionTokenV2.Allow` field is defined lowercase in `.proto` file (`repeated TokenRule allow = 2 [ (gogoproto.jsontag) = "allow,omitempty" ];`). You need to manually patch `types_tfschema.go`, set `Name: "Allow"` in provision token metadata struct (`GenSchemaMetaProvisionTokenV2()`) until it is fixed.

# Usage

See `example/*.tf` for available configuration options. `make apply` to do an initial application of this configuration to your Terraform cluster.

# Testing

`TF_ACC=true` is required to switch Terraform to acceptance test mode. Terraform 1.0.0+ needs to be available on the host machine.

```
TF_ACC=true go test test/*
```

Use '-v' flag to see logs.