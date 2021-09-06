# Teleport Pagerduty Integration

This package provides a Teleport <-> Pagerduty integration that allows you to
treat Teleport access and permission requests as Pagerduty incidents — and
notify the appropriate team, and approve or deny the requests via Pagerduty
special action.

[See setup instructions on Teleport's website](https://goteleport.com/teleport/docs/enterprise/workflow/ssh_approval_pagerduty/)

## Prerequisites

This guide assumes you have

- Teleport Enterprise 6.1 or newer with admin permissions and access to `tctl`
- Pagerduty account already set, with access to creating a new API token.

### Create an access-plugin role and user within Teleport

First off, using an existing Teleport Cluster, we are going to create a new
Teleport User and Role to access Teleport.

#### Create User and Role for access.

Log into Teleport Authent Server, this is where you normally run `tctl`. Don't
change the username and the role name, it should be `access-plugin` for the
plugin to work correctly.

_Note: if you're using other plugins, you might want to create different users
and roles for different plugins_.

```
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
        verbs: ['list','read']
      - resources: ['access_plugin_data']
        verbs: ['update']
    # if you want to enable auto-approve feature
    review_requests:
      roles: ...
      where: ... 
version: v3
EOF

# ...
$ tctl create -f rscs.yaml
```

#### Export access-plugin Certificate

Teleport Plugin uses the `access-plugin`role and user to perform the approval.
We export the identify files, using
[`tctl auth sign`](https://goteleport.com/teleport/docs/cli-docs/#tctl-auth-sign).

```
$ tctl auth sign --format=tls --user=access-plugin --out=auth --ttl=8760h
# ...
```

The above sequence should result in three PEM encoded files being generated:
auth.crt, auth.key, and auth.cas (certificate, private key, and CA certs
respectively). We'll reference these later in the Pagerduty integration config
file.

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

### Setting up Pagerduty API key

In your Pagerduty dashboard, go to **Configuration -> API Access -> Create New
API Key**, add a key description, and save the key. We'll use the key in the
plugin config file later.

### Setting up Pagerduty notification alerts

Once a new access request has been created, plugin creates an incident in a Pagerduty service. In order to know what service to post the notification in, the service name must be set up in a request annotation of a role of a user who requests an access.

Suppose you created a Pagerduty service called "Teleport Notifications" and want it to be notified about all new access requests from users under role `challenger`. Then you should set up a request annotation called `pagerduty_notify_service` containing a list with an only element `["Teleport notifications"]`.

```yaml
kind: role
metadata:
  name: challenger
spec:
  allow:
    request:
      roles: ['champion']
      annotations:
        pagerduty_notify_service: ["teleport notifications"]
```

### Setting up auto-approval behavior

If given sufficient permissions, Pagerduty plugin can auto-approve new access requests if they come from a user who is currently on-call and has at least one active incident assigned to her. More specifically, it works like this:

- Access plugin has an access to submit access reviews:
```yaml
kind: role
metadata:
  name: access-plugin
spec:
  allow:
    # ...
    review_requests:
      roles: ['champion']
      where: ... # If you want to limit the scope of requests the plugin can approve.
      # ...
```
- There's a request annotation called `pagerduty_services` that contains a non-empty list of service names.
```yaml
kind: role
metadata:
  name: challenger
spec:
  allow:
    request:
      roles: ['champion']
      annotations:
        pagerduty_services: ["service 1", "service 2"]
```
- There's a Teleport user with name `alice@example.com` and role `challenger`.
- There's also a Pagerduty user with e-mail `alice@example.com`
- That user is currently on-call in a service "service 1" or "service 2" or in both of them.
- There's at least one active incident assigned to `alice@example.com` in a service where she's currently on-call.
- `alice@example.com` requests a role `champion`.
- Then pagerduty plugin **submits an approval** of Alice's request.

*NOTE* that `pagerduty_services` and `pagerduty_notify_service` annotations should not overlap. You cannot use the same service to post notifications in and be on-call in that service. If `pagerduty_services` and `pagerduty_notify_service` overlap then there'll always be an active incident assigned to user - the notification itself is an incident. So the plugin will auto-approve an access every time which is not actually desired.

## Install

### Installing

```bash

# Check out the repo
git clone https://github.com/gravitational/teleport-plugins.git
cd teleport-plugins

# Build the bot
make access-pagerduty

# Configure the plugin
./access/pagerduty/build/teleport-pagerduty configure > teleport-pagertudy.toml

# Run the plugin, assuming you have teleport running:
./build/teleport-pagerduty start
```

The teleport-pagerduty executable should be placed onto a server that can access
the auth server address.

### Config file

Teleport Pagerduty plugin has its own configuration file in TOML format. Before
starting the plugin for the first time, you'll need to generate and edit that
config file.

```bash
teleport-pagerduty configure > /etc/teleport-pagerduty.toml
```

#### Editing the config file

Afger generating the config, edit it as follows:

```TOML
# example teleport-pagerduty configuration TOML file
[teleport]
addr = "example.com:3025"                                   # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/pagerduty/auth.key" # Teleport GRPC client secret key
client_crt = "/var/lib/teleport/plugins/pagerduty/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/pagerduty/auth.cas"   # Teleport cluster CA certs

[pagerduty]
api_key = "key"               # PagerDuty API Key
user_email = "me@example.com" # PagerDuty bot user email (Could be admin email)

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/pagerduty.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
```

### Running the plugin

```
teleport-pagerduty start
```

By default, `teleport-pagerduty` will assume it's config is in
`/etc/teleport-pagerduty.toml`, but you can override it with `--config` option.
