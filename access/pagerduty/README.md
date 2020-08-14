# Teleport Pagerduty Integration

This package provides a Teleport <-> Pagerduty integration that allows you to
treat Teleport access and permission requests as Pagerduty incidents — and
notify the appropriate team, and approve or deny the requests via Pagerduty
special action.

[See setup instructions on Teleport's website](https://gravitational.com/teleport/docs/enterprise/workflow/ssh_approval_pagerduty/)

## Prerequisites

This guide assumes you have

- Teleport Enterprise 4.2.8 or newer with admin permissions and access to `tctl`
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

Teleport Plugin uses the `access-plugin`role and user to perform the approval.
We export the identify files, using
[`tctl auth sign`](https://gravitational.com/teleport/docs/cli-docs/#tctl-auth-sign).

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

### Setting up Pagerduty API key

In your Pagerduty dashboard, go to **Configuration -> API Access -> Create New
API Key**, add a key description, and save the key. We'll use the key in the
plugin config file later.

### Securing Pagerduty webhooks

Pagerduty doesn't have a mechanism to sign it's webhook payload. Instead, they
provide two good ways for you to verify the integrity and origin of the webhook
requests (i.e. that the webhook is actually sent by Pagerduty, not bo something
else):

- Basic auth (not recommended)
- Certificate verification (recommended and default).

To setup Basic Auth, setup a usename and password in the config below. in
`[http.basic_auth]` section.

If you intend to run `teleport-pagerduty` with TLS anyway, then to ensure mutual
TLS verification, you need to setup `verify-client-cert = true` in the config
below in `[http.tls]` section.

If you're running `teleport-pagerduty` with `--insecure-no-tls`, and another
Proxy server provides TLS certs for your setup, you'll need to setup TLS
verification on that proxy server instead. Pagerduty documentation covers that
process here:
[https://developer.pagerduty.com/docs/webhooks/webhooks-mutual-tls](https://developer.pagerduty.com/docs/webhooks/webhooks-mutual-tls).

## Install

### Installing

```bash

# Check out the repo
git clone git@github.com:gravitational/teleport-plugins.git
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
auth_server = "example.com:3025"                            # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/pagerduty/auth.key" # Teleport GRPC client secret key
client_crt = "/var/lib/teleport/plugins/pagerduty/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/pagerduty/auth.cas"   # Teleport cluster CA certs

[pagerduty]
api_key = "key"               # PagerDuty API Key
user_email = "me@example.com" # PagerDuty bot user email (Could be admin email)
service_id = "PIJ90N7"        # PagerDuty service id

[http]
public_addr = "example.com" # URL on which callback server is accessible externally, e.g. [https://]teleport-pagerduty.example.com
# listen_addr = ":8081" # Network address in format [addr]:port on which callback server listens, e.g. 0.0.0.0:443
https_key_file = "/var/lib/teleport/plugins/pagerduty/server.key"  # TLS private key
https_cert_file = "/var/lib/teleport/plugins/pagerduty/server.crt" # TLS certificate

[http.tls]
verify_client_cert = true # The preferred way to authenticate webhooks on Pagerduty. See more: https://developer.pagerduty.com/docs/webhooks/webhooks-mutual-tls

[http.basic_auth]
user = "user"
password = "password" # If you prefer to use basic auth for Pagerduty Webhooks authentication, use this section to store user and password

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
