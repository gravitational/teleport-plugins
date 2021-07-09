# Teleport Slack Plugin

This package implements a simple Slack plugin using the API provided in the
[`access`](../) package which notifies about Access Requests via Slack
messages.

## Setup

[See setup instructions on Teleport's website](https://goteleport.com/teleport/docs/enterprise/workflow/ssh_approval_slack/)

You must have Go version 1.15 or higher to build.

Run `make access-slack && ./access/slack/build/teleport-slack configure` from
the repository root. The `configure` command will produce an example
configuration file that looks something like this:

```toml
# Example slack plugin configuration TOML file

[teleport]
auth_server = "0.0.0.0:3025"                              # Teleport Auth Server GRPC API address

# tctl auth sign --format=file --auth-server=auth.example.com:3025 --user=access-plugin --out=auth --ttl=1h
identity = "/var/lib/teleport/plugins/slack/auth"         # Teleport certificate ("file" format)

# tctl auth sign --format=tls --auth-server=auth.example.com:3025 --user=access-plugin --out=auth --ttl=1h
# client_key = "/var/lib/teleport/plugins/slack/auth.key" # Teleport GRPC client secret key ("tls" format")
# client_crt = "/var/lib/teleport/plugins/slack/auth.crt" # Teleport GRPC client certificate ("tls" format")
# root_cas = "/var/lib/teleport/plugins/slack/auth.cas"   # Teleport cluster CA certs ("tls" format")

[slack]
token = "xoxb-11xx"                                 # Slack Bot OAuth token
# recipients = ["person@email.com","YYYYYYY"]       # Optional Slack Rooms 
                                                    # Can also set suggested_reviewers for each role

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/slack.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
```

Detailed install steps are provided within the [`install`](INSTALL.md)
instructions.

### `[teleport]`

This configuration section ensures that the bot can talk to your teleport auth
server. Use `tctl auth sign --format=tls` to generate the required PEM files,
and make sure that the Auth Server's GRPC API is accessible at the address
indicated by `auth_server`.

_NOTE_: The slack plugin must be given a teleport user identity with appropriate
permissions. See the [acccess package README](../README.md#authentication) for
an example of how to configure an appropriate user & role.

As this plugin doesn't need the ability to approve or deny requests, you should
enforce read-only behavior by not adding the `update` verb to the plugin user
permissions, like this:

```
  # in the teleport user / role resource yaml
  allow:
    rules:
      - resources: ['access_request']
        verbs: ['list', 'read']
```

### `[slack]`

In order to interact with slack, we need a valid bot OAuth token.

A token can be provisioned from [api.slack.com](https://api.slack.com) by
registering an App and associated Bot User for your workspace.

## Usage

Once your Slack plugin has been configured, you can verify that it's working
correctly by using `tctl request create <user> --roles=<roles>` to simulate an
access request. If everything is working as intended, a message should appear
in the channel specified under `slack.channel`.

Select `Deny` and verify that the request was indeed denied using
`tctl request ls`.

## Security

Currently, this Bot does not make any distinction about _who_ approves/denies a
request. Any user with access to the specified channel will be able to manage
requests. Therefore, it is important that access to the channel be limited.
