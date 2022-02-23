## Teleport Slack Plugin Setup Quickstart

Setup a Teleport Slack Plugin to get notified of
[new teleport access requests](https://goteleport.com/teleport/docs/cli-docs/#tctl-request-ls)
in slack channels and DMs.

For this quickstart, we assume you've already setup a [Teleport cluster](https://goteleport.com/docs/).

## Prerequisites

- A Teleport cluster version 6.1 or later.
- Admin Privileges. With access and control of
  [`tctl`](https://goteleport.com/teleport/docs/cli-docs/#tctl).
- Slack Admin Privileges to create an app and install it to your workspace.

### Authorization

The Teleport Slack Plugin uses the Teleport API *Add link* to connect to a
Teleport Auth Server. In order to have its requests authorized, we need to
create a new User and Role for the plugin.

#### Create User and Role

Log into an existing Teleport Auth server and create a new role and user, both
named `access-plugin`.

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
      # provide read-only permission to access_requests since this plugin
      # does not deal with resolving access requests.
      - resources: ['access_request']
        verbs: ['list', 'read']
      - resources: ['access_plugin_data']
        verbs: ['update']
version: v4
EOF

# ...
$ tctl create -f rscs.yaml
```

#### Export `access-plugin` Identity

The Teleport Slack plugin uses the `access-plugin` user when sending
requests to the Teleport Auth server. To include the user's credentials
in Teleport API requests, we generate TLS and SSH certificates for the 
user, using [`tctl auth sign`](https://goteleport.com/teleport/docs/cli-docs/#tctl-auth-sign).

```bash
# generate user's TLS and SSH certificates into a single identity file.
$ tctl auth sign --format=file --user=access-plugin --out=access-plugin-identity --ttl=8760h
```

The above execution should result in a single identity file named
`access-plugin-identity`. We'll reference this file later when
[configuring Teleport-Plugins](#configuration-file).

_Note: by default, tctl auth sign produces certificates with a relatively short
lifetime. For production deployments, the --ttl flag can be used to ensure a
more practical certificate lifetime. --ttl=8760h exports a 1 year token_

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

You'll need to update the config file with values for your Teleport and
Slack instances.

##### [teleport]

This section is used to connect to your Teleport Auth Server.

`addr`: set this to the your Auth Server address, Reverse Tunnel address,
or Proxy address.

`identity`: set this to the path to the identity file generated in the [export identity](#export-access-plugin-identity) section.
 
#### [slack]

This section is used to connect to your Slack workspace.

`token`: set this to the token found in the [Obtain OAuth Token](#obtain-oauth-token) section

#### [role_to_recipients]

This section is used to configure where the plugin will send access requests
in the Slack workspace.

Provide one or more mappings from a role to recipient(s). Each recipient must be
a slack email or channel. Example:
```toml
[role_to_recipients]
"*" = ["admin@email.com", "admin-slack-channel"]
"dev" = "dev-slack-channel"
```

A wildcard `*` entry must be provided to ensure that every role can be mapped
to recipients.

#### [log]

This section can be changed to change the plugin's logging attributes.

`output`: where to output logs - can be set to `stdout`, `stderr`, or a log folder.

`severity`: what severity to log with - can be `INFO`, `ERROR`, `DEBUG`, or `WARN`.

In Slack section, use the OAuth token provided by Slack.

## Test Run

Assuming that Teleport is running, and you've created the Slack app, the plugin
config, and provided all the certificates — you can now run the plugin and test
the workflow!

`teleport-slack start`

The log output should look like this:

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
