# Teleport Mattermost Bot

This package provides Teleport <-> Mattermost integrataion that allows teams to
approve or deny Teleport access requests using Mattermost.

## Setup

[See setup instructions on Teleport's website](https://gravitational.com/teleport/docs/enterprise/workflow/ssh_approval_mattermost/)

### Prerequisites

This guide assumes that you have:

- Teleport Enterprise 4.2.8 or newer
- Admin privileges with access to `tctl`
- Mattermost account with admin privileges.

#### Setting up a sandbox Mattermost instance for testing

If you want to build the plugin and test it with Mattermost, the easiest way to
get Mattermost running is with the docker image:

```bash
docker run --name mattermost-preview -d --publish 8065:8065 --add-host dockerhost:127.0.0.1 mattermost/mattermost-preview
```

Check out
[more documentation on running Mattermost](https://docs.mattermost.com/install/docker-local-machine.html).

#### Setting up Mattermost to work with the bot

In Mattermost, go to System Console -> Integrations -> Enable Bot Account
Creation -> Set to True. This will allow us to create a new bot account that the
Teleport bot will use.

Go back to your team, then Integrations -> Bot Accounts -> Add Bot Account.

The new bot account will need Post All permission.

The confirmation screen after you've created the bot will give you the access
token. We'll use this in the config later.

#### Create an access-plugin role and user within Teleport

First off, using an existing Teleport Cluster, we are going to create a new
Teleport User and Role to access Teleport.

#### Create User and Role for access.

Log into Teleport Authentication Server, this is where you normally run `tctl`.
Don't change the username and the role name, it should be `access-plugin` for
the plugin to work correctly.

```bash
$ cat > rscs.yaml <<EOF
kind: user
metadata:
  name: access-plugin
spec:
  roles: ['access-plugin']
version: v2
---
kind: role
metadata:
  name: access-plugin
spec:
  allow:
    rules:
      - resources: ['access_request']
        verbs: ['list','read','update']
    # teleport currently refuses to issue certs for a user with 0 logins,
    # this restriction may be lifted in future versions.
    logins: ['access-plugin']
version: v3
EOF

# ...
$ tctl create -f rscs.yaml
```

#### Export access-plugin Certificate

Teleport Plugin uses the `access-plugin`role and user to peform the approval. We
export the identify files, using
[`tctl auth sign`](https://gravitational.com/teleport/docs/cli-docs/#tctl-auth-sign).

```bash
$ tctl auth sign --format=tls --user=access-plugin --out=auth --ttl=8760h
# ...
```

The above sequence should result in three PEM encoded files being generated:
auth.crt, auth.key, and auth.cas (certificate, private key, and CA certs
respectively). We'll reference these later in the bot config, and move them to
an appropriate directory.

_Note: by default, tctl auth sign produces certificates with a relatively short
lifetime. For production deployments, the --ttl flag can be used to ensure a
more practical certificate lifetime. --ttl=8760h exports a 1 year token_

## Downloading and installing the plugin

The recommended way to run Teleport Mattermost plugin is by downloading the
release version and installing it:

```bash
$ wget https://get.gravitational.com/teleport-mattermost-v0.0.1-linux-amd64-bin.tar.gz
$ tar -xzf teleport-mattermost-v0.0.1-linux-amd64-bin.tar.gz
$ cd teleport-mattermost
$ ./install
$ which teleport-mattermost
/usr/local/bin/teleport-mattermost
```

### Building from source

```bash

# Checkout teleport-plugins
git clone git@github.com:gravitational/teleport-plugins.git
cd teleport-plugins

cd access/mattermost
make
```

### Configuring Mattermost bot

Mattermost Bot uses a config file in TOML format. Generate a boilerplate config
by running the following command:

```
teleport-mattermost configure > /etc/teleport-mattermost.yml
```

Then, edit the config as needed.

```TOML
# example mattermost configuration TOML file
[teleport]
auth_server = "example.com:3025"  # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/mattermost/auth.key" # Teleport GRPC client secret key
client_crt = "/var/lib/teleport/plugins/mattermost/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/mattermost/auth.cas"   # Teleport cluster CA certs

[mattermost]
url = "https://mattermost.example.com" # Mattermost Server URL
team = "team-name"                     # Mattermsot team in which the channel resides.
channel = "channel-name"               # Mattermost Channel name to post requests to
token = "api-token"                    # Mattermost Bot OAuth token
secret = "signing-secret-value"        # Mattermost API signing Secret

[http]
public_addr = "example.com" # URL on which callback server is accessible externally, e.g. [https://]teleport-mattermost.example.com
# listen_addr = ":8081" # Network address in format [addr]:port on which callback server listens, e.g. 0.0.0.0:443
https_key_file = "/var/lib/teleport/plugins/mattermost/server.key"  # TLS private key
https_cert_file = "/var/lib/teleport/plugins/mattermost/server.crt" # TLS certificate

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/mattermost.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
```

### Running the bot

With the config above, you should be able to run the bot invoking
`teleport-mattermost start`
