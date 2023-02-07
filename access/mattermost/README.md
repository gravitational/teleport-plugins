# Teleport Mattermost Plugin

This package provides Teleport <-> Mattermost integrataion that allows teams to
get notified about new access requests in Mattermost.

[See setup instructions on Teleport's website](https://goteleport.com/teleport/docs/enterprise/workflow/ssh_approval_mattermost/)

## Prerequisites

This guide assumes that you have:

- Teleport 6.1.0 or newer
- Admin privileges with access to `tctl`
- Mattermost account with admin privileges.

## Install the plugin

There are several methods to installing and using the Teleport Mattermost Plugin:

1. Use a [precompiled binary](#precompiled-binary)

2. Use a [docker image](#docker-image)

3. Install from [source](#building-from-source)

### Precompiled Binary

Get the plugin distribution.

```bash
$ curl -L https://get.gravitational.com/teleport-access-mattermost-v7.0.2-linux-amd64-bin.tar.gz
$ tar -xzf teleport-access-mattermost-v7.0.2-linux-amd64-bin.tar.gz
$ cd teleport-access-mattermost
$ ./install
```

### Docker Image
```bash
$ docker pull public.ecr.aws/gravitational/teleport-plugin-mattermost:9.0.2
```

```bash
$ docker run public.ecr.aws/gravitational/teleport-plugin-mattermost:9.0.2 version
teleport-mattermost v9.0.2 git:teleport-mattermost-v9.0.2-0-g9e149895 go1.17.8
```

For a list of available tags, visit [Amazon ECR Public Gallery](https://gallery.ecr.aws/gravitational/teleport-plugin-mattermost)

### Building from source

To build the plugin from source you need [Go](https://go.dev/) and `make`.

```bash
$ git clone https://github.com/gravitational/teleport-plugins.git
$ cd teleport-plugins/access/mattermost
$ make
$ ./build/teleport-mattermost start
```

## Setting up a sandbox Mattermost instance for testing

If you want to build the plugin and test it with Mattermost, the easiest way to
get Mattermost running is with the docker image:

```bash
docker run --name mattermost-preview -d --publish 8065:8065 --add-host dockerhost:127.0.0.1 mattermost/mattermost-preview
```

Check out
[more documentation on running Mattermost](https://docs.mattermost.com/install/docker-local-machine.html).

### Setting up Mattermost to work with the plugin

In Mattermost, go to System Console -> Integrations -> Enable Bot Account
Creation -> Set to True. This will allow us to create a new bot account that the
Teleport plugin will use.

Go back to your team, then Integrations -> Bot Accounts -> Add Bot Account.

The new bot account will need Post All permission.

The confirmation screen after you've created the bot will give you the access
token. We'll use this in the config later.

## Teleport User and Role

Using Web UI or `tctl` CLI utility, create the role `access-mattermost` and the user `access-mattermost` belonging to the role `access-mattermost`. You may use the following YAML declarations.

### Role

```yaml
kind: role
metadata:
  name: access-mattermost
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
  name: access-mattermost
spec:
  roles: ['access-mattermost']
version: v2
```

## Generate the certificate

For the plugin to connect to Auth Server, it needs an identity file containing TLS/SSH certificates. This can be obtained with tctl:

```bash
$ tctl auth sign --auth-server=AUTH-SERVER:PORT --format=file --user=access-mattermost --out=/var/lib/teleport/plugins/mattermost/auth_id --ttl=8760h
```

Here, `AUTH-SERVER:PORT` could be `localhost:3025`, `your-in-cluster-auth.example.com:3025`, `your-remote-proxy.example.com:3080` or `your-teleport-cloud.teleport.sh:443`. For non-localhost connections, you might want to pass the `--identity=...` option to authenticate yourself to Auth Server.

## Configuring Mattermost Plugin

Mattermost Plugin uses a config file in TOML format. Generate a boilerplate config
by running the following command:

```
$ teleport-mattermost configure > /etc/teleport-mattermost.yml
```

Then, edit the config as needed.

```TOML
# example mattermost configuration TOML file
[teleport]
# Teleport Auth/Proxy Server address.
#
# Should be port 3025 for Auth Server and 3080 or 443 for Proxy.
# For Teleport Cloud, should be in the form "your-account.teleport.sh:443".
addr = "example.com:3025"

# Credentials.
#
# When using --format=file:
# identity = "/var/lib/teleport/plugins/mattermost/auth_id"    # Identity file
#
# When using --format=tls:
# client_key = "/var/lib/teleport/plugins/mattermost/auth.key" # Teleport TLS secret key
# client_crt = "/var/lib/teleport/plugins/mattermost/auth.crt" # Teleport TLS certificate
# root_cas = "/var/lib/teleport/plugins/mattermost/auth.cas"   # Teleport CA certs

[mattermost]
url = "https://mattermost.example.com" # Mattermost Server URL
token = "api-token"                    # Mattermost Bot OAuth token

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/mattermost.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
```

## Running the plugin

With the config above, you should be able to run the bot invoking

```bash
$ teleport-mattermost start
```

or with docker:

```bash
$ docker run -v <path/to/config>:/etc/teleport-mattermost.toml public.ecr.aws/gravitational/teleport-plugin-mattermost:9.0.2 start
```

## The Workflow

### Create an access request

You can create an access request using Web UI going to
`https://your-proxy.example.com/web/requests/new` where your-proxy.example.com
is your Teleport Proxy public address. There you should specify the reviewers
whose usernames *must match the emails of Mattermost users* which you want to be notified.
Check that you see a request message on Mattermost.

It should look like this: %image%

### Review the request

Open the Link in message and choose to either approve or deny the request. The messages should automatically get updated to reflect the action you just did.

## Teleport OSS edition

Currently, Teleport OSS edition does not have an "Access Requests" page at Web UI. Alternatively, you can create an access request using tsh:

```bash
tsh request create --roles=foo --reviewers=some-user@example.com

98afcb7d-9c6d-4a8f-8a03-9124fbbcb059
```

*Note:* There must be a user with an email `some-user@example.com` registered in your Mattermost team.

To approve or deny the request:

```bash
tsh request review --approve 98afcb7d-9c6d-4a8f-8a03-9124fbbcb059
tsh request review --deny 98afcb7d-9c6d-4a8f-8a03-9124fbbcb059
```
