# Teleport Slack Plugin

This package implements a simple Slack plugin using the Teleport Access API. A slack channel receives an alert when an access request is created.

## Setup

[See setup instructions on Teleport's website](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-slack/)

Detailed install steps are provided in our [docs](https://goteleport.com/docs/enterprise/workflow/ssh-approval-slack/).

## Install the plugin

There are several methods to installing and using the Teleport Slack Plugin:

1. Use a [precompiled binary](#precompiled-binary)

2. Use a [docker image](#docker-image)

3. Install from [source](#building-from-source)

### Precompiled Binary

Get the plugin distribution.

```bash
$ curl -L https://get.gravitational.com/teleport-access-slack-v7.0.2-linux-amd64-bin.tar.gz
$ tar -xzf teleport-access-slack-v7.0.2-linux-amd64-bin.tar.gz
$ cd teleport-access-slack
$ ./install
```

### Docker Image
```bash
$ docker pull public.ecr.aws/gravitational/teleport-plugin-slack:9.0.2
```

```bash
$ docker run public.ecr.aws/gravitational/teleport-plugin-slack:9.0.2 version
teleport-slack v9.0.2 git:teleport-slack-v9.0.2-0-g9e149895 go1.17.8
```

For a list of available tags, visit [Amazon ECR Public Gallery](https://gallery.ecr.aws/gravitational/teleport-plugin-slack)

### Building from source

To build the plugin from source you need [Go](https://go.dev/) and `make`.

```bash
$ git clone https://github.com/gravitational/teleport-plugins.git
$ cd teleport-plugins/access/slack
$ make
$ ./build/teleport-slack start
```



## Teleport User and Role

Using Web UI or `tctl` CLI utility, create the role `access-slack` and the user `access-slack` belonging to the role `access-slack`. You may use the following YAML declarations.

### Role

```yaml
kind: role
metadata:
  name: access-slack
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
  name: access-slack
spec:
  roles: ['access-slack']
version: v2
```

## Generate the certificate

For the plugin to connect to Auth Server, it needs an identity file containing TLS/SSH certificates. This can be obtained with tctl:

```bash
$ tctl auth sign --auth-server=AUTH-SERVER:PORT --format=file --user=access-slack --out=/var/lib/teleport/plugins/slack/auth_id --ttl=8760h
```

Here, `AUTH-SERVER:PORT` could be `localhost:3025`, `your-in-cluster-auth.example.com:3025`, `your-remote-proxy.example.com:3080` or `your-teleport-cloud.teleport.sh:443`. For non-localhost connections, you might want to pass the `--identity=...` option to authenticate yourself to Auth Server.

## Configuring Slack Plugin

Slack Plugin uses a config file in TOML format. Generate a boilerplate config
by running the following command:

```
$ teleport-slack configure > /etc/teleport-slack.yml
```

Then, edit the config as needed.

```TOML
# Example slack plugin configuration TOML file

[teleport]
# Teleport Auth/Proxy Server address.
# addr = "example.com:3025"
#
# Should be port 3025 for Auth Server and 3080 or 443 for Proxy.
# For Teleport Cloud, should be in the form "your-account.teleport.sh:443".

# Credentials generated with `tctl auth sign`.
#
# When using --format=file:
# identity = "/var/lib/teleport/plugins/slack/auth_id"    # Identity file
#
# When using --format=tls:
# client_key = "/var/lib/teleport/plugins/slack/auth.key" # Teleport TLS secret key
# client_crt = "/var/lib/teleport/plugins/slack/auth.crt" # Teleport TLS certificate
# root_cas = "/var/lib/teleport/plugins/slack/auth.cas"   # Teleport CA certs

[slack]
# Slack Bot OAuth token
token = "xoxb-11xx"

[role_to_recipients]
# Map roles to recipients.
#
# Provide slack user_email/channel recipients for access requests for specific roles. 
# role.suggested_reviewers will automatically be treated as additional email recipients.
# "*" must be provided to match non-specified roles.
#
# "dev" = "devs-slack-channel"
# "*" = ["admin@email.com", "admin-slack-channel"]

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/slack.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
```

## Running the plugin

With the config above, you should be able to run the bot invoking

```bash
$ teleport-slack start
```

or with docker:

```bash
$ docker run -v <path/to/config>:/etc/teleport-slack.toml public.ecr.aws/gravitational/teleport-plugin-slack:9.0.2 start
```

## Usage

Once your Slack plugin has been configured, you can verify that it's working
correctly by using `tctl request create <user> --roles=<roles>` to simulate an
access request. If everything is working as intended, a message should appear
in the channel specified under `slack.channel`.

Select `Deny` and verify that the request was indeed denied using
`tctl request ls`.
