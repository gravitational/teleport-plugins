## Teleport Slack Plugin Setup Quickstart

If you're using Slack, you can be notified of
[new teleport access requests](https://goteleport.com/teleport/docs/cli-docs/#tctl-request-ls).
This guide covers it's setup.

For this quickstart, we assume you've already setup a [Teleport cluster](https://goteleport.com/docs/).

## Prerequisites

- A Teleport cluster version 6.1 or later.
- Admin Privileges. With access and control of
  [`tctl`](https://goteleport.com/teleport/docs/cli-docs/#tctl)
- Slack Admin Privileges to create an app and install it to your workspace.

### Create an access-plugin role and user within Teleport

First off, using an existing Teleport cluster, we are going to create a new
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
        verbs: ['list', 'read']
      - resources: ['access_plugin_data']
        verbs: ['update']
    # teleport currently refuses to issue certs for a user with 0 logins,
    # this restriction may be lifted in future versions.
    logins: ['access-plugin']
version: v3
EOF

# ...
$ tctl create -f rscs.yaml
```

#### Export access-plugin Certificate

Teleport Plugin uses the `access-plugin` role and user to perform the approval.
We export the identify files, using
[`tctl auth sign`](https://goteleport.com/teleport/docs/cli-docs/#tctl-auth-sign).

```bash
$ tctl auth sign --format=tls --user=access-plugin --out=auth --ttl=8760h
# ...
```

The above sequence should result in three PEM encoded files being generated:
auth.crt, auth.key, and auth.cas (certificate, private key, and CA certs
respectively). We'll reference these later when
[configuring Teleport-Plugins](#configuration-file), and move them to an
appropriate directory.

_Note: by default, tctl auth sign produces certificates with a relatively short
lifetime. For production deployments, the --ttl flag can be used to ensure a
more practical certificate lifetime. --ttl=8760h exports a 1 year token_

#### Export access-plugin Certificate for use with Teleport Cloud

Connection to Teleport Cloud is only possible with reverse tunnel. For this reason,
we need the identity signed in a different format called `file` which exports
SSH keys too.

```bash
$ tctl auth sign --auth-server=yourproxy.teleport.sh:443 --format=file --user=access-plugin --out=auth --ttl=8760h
# ...
```

### Create Slack App

We'll create a new Slack app and setup auth token. You'll need to:

1. Create a new app, pick a name and select a workspace it belongs to.
2. Add OAuth Scopes. This is required by Slack for the app to be installed —
   we'll only need a single scope to post messages to your Slack account.
3. Obtain OAuth token for the Teleport plugin config.

#### Creating the app

https://api.slack.com/apps

App Name: Teleport Development Slack Workspace: Pick the workspace you'd like
the requests to show up in.

![Create Slack App](https://p197.p4.n0.cdn.getcloudapp.com/items/llu4EL7e/Image+2020-01-09+at+10.40.39+AM.png?v=d9750e4fdc77901e0c2ffb2dc6040aee)

#### Selecting OAuth Scopes

On the App screen, go to “OAuth and Permissions” under Features in the sidebar
menu. Then scroll to Scopes, and add `chat:write`, `users:read` and
`users:read.email` scopes so that our plugin can post messages to your Slack
and query the user information.

#### Install to Workspace

![OAuth Token](https://p197.p4.n0.cdn.getcloudapp.com/items/E0uEg1ol/Image+2020-01-09+at+11.00.23+AM.png?v=1e28ff5bc4f7e0754acc9c7823f354a3)

#### Obtain OAuth Token

![OAuth Token](images/oauth.png)

## Installing the Teleport Slack Plugin

To start using Teleport Plugins, you will need the teleport-slack executable.
See the [README](README.md) for building the teleport-slack executable in the
Setup section. Place the executable in the appropriate /usr/bin or
/usr/local/bin on the server installation.

### Configuration File

Teleport Slack plugin has its own config file in TOML format. Before starting
the plugin, you'll need to generate (or just copy the one below) and edit that
config.

To generate a config file, you can do this:

```bash
teleport-slack configure > /etc/teleport-slack.toml
```

Note that it saves the config file to `/etc/teleport-slack.toml`. You'll be able
to point the plugin to any config file path you want, but it'll pick up
`/etc/teleport-slack.toml` by default.

#### Editing the config file

In the Teleport section, use the certificates you've generated with
`tctl auth sign` before. The plugin installer creates a folder for those
certificates in `/var/lib/teleport/plugins/slack/` — so just move the
certificates there and make sure the config points to them.

In Slack section, use the OAuth token provided by Slack.

```TOML
# Example Teleport Slack Plugin config file
[teleport]
auth_server = "example.com:3025"  # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/slack/auth.key" # Teleport GRPC client secret key
client_crt = "/var/lib/teleport/plugins/slack/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/slack/auth.cas"   # Teleport cluster CA certs

[slack]
token = "api-token"       # Slack Bot OAuth token

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/slack.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
```

To use with Teleport Cloud, you should set a path to identity file exported with `--format=file` option.

```TOML
[teleport]
auth_server = "yourproxy.teleport.sh"             # Teleport proxy address
identity = "/var/lib/teleport/plugins/slack/auth" # Teleport identity file
```

## Test Run

Assuming that Teleport is running, and you've created the Slack app, the plugin
config, and provided all the certificates — you can now run the plugin and test
the workflow!

`teleport-slack start`

If everything works fine, the log output should look like this:

```bash
vm0:~/slack sudo ./teleport=slack start
INFO   Starting Teleport Access Slack Plugin 6.1.0: slack/main.go:224
INFO   Starting a request watcher... slack/main.go:330
INFO   Watcher connected slack/main.go:298
```

### The Workflow

#### Create an access request

You can create an access request using Web UI going to
`https://your-proxy.example.com/web/requests/new` where `your-proxy.example.com`
is your Teleport Proxy public address. There you should specify the reviewers
whose usernames *must match the emails of Slack users* which you want to be
notified.

#### Check that you see a request message on Slack

It should look like this: %image%

#### Review the request

Open the **Link** in message and choose to either approve or deny the request.
The messages should automatically get updated to reflect the action you just
did.

### Teleport OSS edition

Currently, Teleport OSS edition does not have an "Access Requests" page at Web
UI. Alternatively, you can create an access request using tsh:

```bash
tsh request create --roles=foo --reviewers=some-user@example.com

98afcb7d-9c6d-4a8f-8a03-9124fbbcb059
```

*Note:* There must be a user with an email `some-user@example.com` registered in
your Slack workspace.

To approve or deny the request:

```bash
tsh request review --approve 98afcb7d-9c6d-4a8f-8a03-9124fbbcb059
tsh request review --deny 98afcb7d-9c6d-4a8f-8a03-9124fbbcb059
```

### Setup with systemd

In production, we recommend starting teleport plugin daemon via an init system
like [systemd](https://systemd.io/). Here's the recommended Teleport Plugin service unit file for
systemd:

```
[Unit]
Description=Teleport Slack Plugin
After=network.target

[Service]
Type=simple
Restart=on-failure
ExecStart=/usr/local/bin/teleport-slack start --config=/etc/teleport-slack.toml --pid-file=/var/run/teleport-slack.pid
ExecReload=/bin/kill -HUP $MAINPID
PIDFile=/var/run/teleport-slack.pid

[Install]
WantedBy=multi-user.target
```

Save this as `teleport-slack.service`.

# FAQ / Debugging
