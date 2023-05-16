## Provider configuration

Ensure your Terraform version is v(=terraform.version=) or higher.

Add the following configuration section to your `terraform` configuration block:

```
terraform {
  required_providers {
    teleport = {
      version = ">= (=teleport.version=)"
      source  = "terraform.releases.teleport.dev/gravitational/teleport"
    }
  }
}
```

The provider supports the following options:

| Name                   | Type   | Description                                                                    | Environment Variable               |
| ---------------------- | ------ | ------------------------------------------------------------------------------ | ---------------------------------- |
| `addr`                 | string | Teleport auth or proxy address in "host:port" format.                          | `TF_TELEPORT_ADDR`                 |
| `cert_path`            | string | Path to Teleport certificate file.                                             | `TF_TELEPORT_CERT`                 |
| `cert_base64`          | string | Teleport certificate as base64.                                                | `TF_TELEPORT_CERT_BASE64`          |
| `identity_file_path`   | string | Path to Teleport identity file.                                                | `TF_TELEPORT_IDENTITY_FILE_PATH`   |
| `identity_file_base64` | string | Teleport identity file as base64.                                              | `TF_TELEPORT_IDENTITY_FILE_BASE64` |
| `key_path`             | string | Path to Teleport key file.                                                     | `TF_TELEPORT_KEY`                  |
| `key_base64`           | string | Teleport key as base64.                                                        | `TF_TELEPORT_KEY_BASE64`           |
| `profile_dir`          | string | Teleport profile path.                                                         | `TF_TELEPORT_PROFILE_PATH`         |
| `profile_name`         | string | Teleport profile name.                                                         | `TF_TELEPORT_PROFILE_NAME`         |
| `root_ca_path`         | string | Path to Teleport CA file.                                                      | `TF_TELEPORT_ROOT_CA`              |
| `root_ca_base64`       | string | Teleport CA as base64.                                                         | `TF_TELEPORT_ROOT_CA_BASE64`       |
| `retry_base_duration`  | string | Base duration between retries. [Format](https://pkg.go.dev/time#ParseDuration) | `TF_TELEPORT_RETRY_BASE_DURATION`  |
| `retry_cap_duration`   | string | Max duration between retries. [Format](https://pkg.go.dev/time#ParseDuration)  | `TF_TELEPORT_RETRY_CAP_DURATION`   |
| `retry_max_tries`      | string | Max number of retries.                                                         | `TF_TELEPORT_RETRY_MAX_TRIES`      |

You need to specify at least one of:

- `cert_path`, `key_path`,`root_ca_path` and `addr` to connect using key files.
- `cert_base64`, `key_base64`,`root_ca_base64` and `addr` to connect using a base64-encoded key.
- `identity_file_path` or `identity_file_base64` and `addr` to connect using an identity file.
- `profile_name`, `profile_dir` (both can be empty) and `addr` to connect using current profile from `~/.tsh`

The `retry_*` values are used to retry the API calls to Teleport when the cache is stale.

If more than one are provided, they will be tried in the order above until one succeeds.

Example:

```
provider "teleport" {
  addr         = "localhost:3025"
  cert_path    = "tf.crt"
  key_path     = "tf.key"
  root_ca_path = "tf.ca"
}
```