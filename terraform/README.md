# Terraform Provider Plugin

# Installation

1. Clone the plugin:

```bash
git clone git@github.com:gravitational/teleport-plugins
```

_NOTE: This URL will be changed after merge_

2. Install the plugin to Teleport:

```bash
cd teleport-plugins/terraform
make build
```

3. Configure teleport:

```bash
tctl create example/teleport.yaml
tctl auth sign --format=tls --user=terraform --out=tf --ttl=10h
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

# Usage

See `example/main.tf` for available configuration options. `make apply` to do an initial application of this configuration to your Terraform cluster.

# TODO

- [ ] Data Sources (if applicable, needs discussing)
