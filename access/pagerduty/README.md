# Teleport Pagerduty Integration

This package provides a Teleport <-> Pagerduty integration that allows you to
treat Teleport access and permission requests as Pagerduty incidents — and
notify the appropriate team, and approve or deny the requests via Pagerduty
special action.

[See setup instructions on Teleport's website](https://goteleport.com/teleport/docs/enterprise/workflow/ssh-approval-pagerduty/)

## Prerequisites

This guide assumes you have

- Teleport Enterprise 6.1 or newer with admin permissions and access to `tctl`
- Pagerduty account already set, with access to creating a new API token.

## Install the plugin

There are several methods to installing and using the Teleport Pagerduty Plugin:

1. Use a [precompiled binary](#precompiled-binary)

2. Use a [docker image](#docker-image)

3. Install from [source](#building-from-source)

### Precompiled Binary

Get the plugin distribution.

```bash
$ curl -L https://get.gravitational.com/teleport-access-pagerduty-v7.0.2-linux-amd64-bin.tar.gz
$ tar -xzf teleport-access-pagerduty-v7.0.2-linux-amd64-bin.tar.gz
$ cd teleport-access-pagerduty
$ ./install
```

### Docker Image
```bash
$ docker pull public.ecr.aws/gravitational/teleport-plugin-pagerduty:9.0.2
```

```bash
$ docker run public.ecr.aws/gravitational/teleport-plugin-pagerduty:9.0.2 version
teleport-pagerduty v9.0.2 git:teleport-pagerduty-v9.0.2-0-g9e149895 go1.17.8
```

For a list of available tags, visit [Amazon ECR Public Gallery](https://gallery.ecr.aws/gravitational/teleport-plugin-pagerduty)

### Building from source

To build the plugin from source you need [Go](https://go.dev/) and `make`.

```bash
$ git clone https://github.com/gravitational/teleport-plugins.git
$ cd teleport-plugins/access/pagerduty
$ make
$ ./build/teleport-pagerduty start
```

## Teleport User and Role

Using Web UI or `tctl` CLI utility, create the role `access-pagerduty` and the user `access-pagerduty` belonging to the role `access-pagerduty`. You may use the following YAML declarations.

### Role

```yaml
kind: role
metadata:
  name: access-pagerduty
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
  name: access-pagerduty
spec:
  roles: ['access-pagerduty']
version: v2
```

## Generate the certificate

For the plugin to connect to Auth Server, it needs an identity file containing TLS/SSH certificates. This can be obtained with tctl:

```bash
$ tctl auth sign --auth-server=AUTH-SERVER:PORT --format=file --user=access-pagerduty --out=/var/lib/teleport/plugins/pagerduty/auth_id --ttl=8760h
```

Here, `AUTH-SERVER:PORT` could be `localhost:3025`, `your-in-cluster-auth.example.com:3025`, `your-remote-proxy.example.com:3080` or `your-teleport-cloud.teleport.sh:443`. For non-localhost connections, you might want to pass the `--identity=...` option to authenticate yourself to Auth Server.

## Setting up Pagerduty API key

In your Pagerduty dashboard, go to **Configuration -> API Access -> Create New
API Key**, add a key description, and save the key. We'll use the key in the
plugin config file later.

## Setting up Pagerduty notification alerts

Once a new access request has been created, the plugin creates an incident in a Pagerduty service. In order to know what service to post the notification in, the service name must be set up in a request annotation of a role of a user who requests an access.

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

## Setting up auto-approval behavior

If given sufficient permissions, Pagerduty plugin can auto-approve new access requests if they come from a user who is currently on-call. More specifically, it works like this:

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
- `alice@example.com` requests a role `champion`.
- Then pagerduty plugin **submits an approval** of Alice's request.


## Configuring Pagerduty Plugin

Teleport Pagerduty plugin has its own configuration file in TOML format. Before
starting the plugin for the first time, you'll need to generate and edit that
config file.

```bash
$ teleport-pagerduty configure > /etc/teleport-pagerduty.toml
```

Then, edit the config as needed.

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

## Running the plugin

By default, `teleport-pagerduty` will assume it's config is in
`/etc/teleport-pagerduty.toml`, but you can override it with `--config` option.

```
$ teleport-pagerduty start
```

or with docker:

```bash
$ docker run -v <path/to/config>:/etc/teleport-pagerduty.toml public.ecr.aws/gravitational/teleport-plugin-pagerduty:9.0.2 start
```
