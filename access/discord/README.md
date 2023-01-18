# Teleport Discord Plugin

This package implements a simple Discord plugin using the Teleport Access API. A discord channel receives an alert when an access request is created.

## Setup

[See setup instructions on Teleport's website](https://goteleport.com/teleport/docs/enterprise/workflow/ssh_approval_discord/)

Detailed install steps are provided in our [docs](https://goteleport.com/docs/enterprise/workflow/ssh-approval-discord/).

## Install the plugin

There are several methods to installing and using the Teleport Discord Plugin:

1. Use a [precompiled binary](#precompiled-binary)

2. Use a [docker image](#docker-image)

3. Install from [source](#building-from-source)

### Precompiled Binary

Get the plugin distribution.

```bash
$ curl -L https://get.gravitational.com/teleport-access-discord-v7.0.2-linux-amd64-bin.tar.gz
$ tar -xzf teleport-access-discord-v11.1.0-linux-amd64-bin.tar.gz
$ cd teleport-access-discord
$ ./install
```

### Docker Image
```bash
$ docker pull public.ecr.aws/gravitational/teleport-plugin-discord:11.1.0
```

```bash
$ docker run public.ecr.aws/gravitational/teleport-plugin-discord:11.1.0 version
teleport-discord v11.1.0 git:teleport-discord-v11.1.0-0-g9e149895 go1.19.1
```

For a list of available tags, visit [Amazon ECR Public Gallery](https://gallery.ecr.aws/gravitational/teleport-plugin-discord)

### Building from source

To build the plugin from source you need [Go](https://go.dev/) and `make`.

```bash
$ git clone https://github.com/gravitational/teleport-plugins.git
$ cd teleport-plugins/access/discord
$ make
$ ./build/teleport-discord start
```

## Teleport User and Role

Using Web UI or `tctl` CLI utility, create the role `access-discord` and the user `access-discord` belonging to the role `access-discord`. You may use the following YAML declarations.

### Role

```yaml
kind: role
metadata:
  name: access-discord
spec:
  allow:
    rules:
      - resources: ['access_request']
        verbs: ['list', 'read', 'update']
version: v6
```

### User

```yaml
kind: user
metadata:
  name: access-discord
spec:
  roles: ['access-discord']
version: v2
```

## Generate the certificate

For the plugin to connect to Auth Server, it needs an identity file containing TLS/SSH certificates. This can be obtained with tctl:

```bash
$ tctl auth sign --auth-server=AUTH-SERVER:PORT --format=file --user=access-discord --out=/var/lib/teleport/plugins/discord/auth_id --ttl=8760h
```

Here, `AUTH-SERVER:PORT` could be `localhost:3025`, `your-in-cluster-auth.example.com:3025`, `your-remote-proxy.example.com:3080` or `your-teleport-cloud.teleport.sh:443`. For non-localhost connections, you might want to pass the `--identity=...` option to authenticate yourself to Auth Server.

## Configuring Discord Plugin

Discord Plugin uses a config file in TOML format. Generate a boilerplate config
by running the following command:

```
$ teleport-discord configure > /etc/teleport-discord.yml
```

Then, edit the config as needed.

```TOML
# Example discord plugin configuration TOML file

[teleport]
# Teleport Auth/Proxy Server address.
# addr = "example.com:3025"
#
# Should be port 3025 for Auth Server and 3080 or 443 for Proxy.
# For Teleport Cloud, should be in the form "your-account.teleport.sh:443".

# Credentials generated with `tctl auth sign`.
#
# When using --format=file:
# identity = "/var/lib/teleport/plugins/discord/auth_id"    # Identity file
#
# When using --format=tls:
# client_key = "/var/lib/teleport/plugins/discord/auth.key" # Teleport TLS secret key
# client_crt = "/var/lib/teleport/plugins/discord/auth.crt" # Teleport TLS certificate
# root_cas = "/var/lib/teleport/plugins/discord/auth.cas"   # Teleport CA certs

[discord]
token = "my-token"

[role_to_recipients]
# Map roles to recipients.
#
# Provide discord channelID recipients for access requests for specific roles. 
# "*" must be provided to match non-specified roles.
#
# "dev" = ["1234567890","0987654321"]
# "*" = ["1234567890"]

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/discord.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
```

## Running the plugin

With the config above, you should be able to run the bot invoking

```bash
$ teleport-discord start
```

or with docker:

```bash
$ docker run -v <path/to/config>:/etc/teleport-discord.toml public.ecr.aws/gravitational/teleport-plugin-discord:11.1.0 start
```

## Usage

Once your Discord plugin has been configured, you can verify that it's working
correctly by using `tctl request create <user> --roles=<roles>` to simulate an
access request. If everything is working as intended, a message should appear
in the channel specified under `discord.channel`.

Select `Deny` and verify that the request was indeed denied using
`tctl request ls`.
