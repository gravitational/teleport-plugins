# teleport-webhooks

`teleport-webhooks` is a teleport plugin that allows you to send a webhook to a
preconfigured URL with the access request meta information every time the access
request is created or updated (approved/denied) or deleted, and, optionally,
receive and handle HTTP requests to perform updates on access requests
(approvals/denials).

We created this plugin to make it easy for anyone to extend teleport with any
behavior they want, and learn how to create their own plugins when the existing
plugins don't work for their scenarios.

With `teleport-webhooks` you can:

- Roll your own authentication hooks system with a thin wrapper around your
  authorization management logic.
- Use requestbin to debug what information gets sent out on a request update
- Play around with Zapier or other similar systems.

## Security

**Warning: this plugin allows you to send and manage access requests via HTTP
using any 3rd party service. Using public SaaS or productivity services may
significantly reduce the overall security of your authentication system. Be
extra cautious setting this up.**

Basic security guidelines using `teleport-webhooks`:

- Don't send webhooks over plain HTTP, always use HTTPS hook URLS.
- Don't use publicly available SaaS services that you don't control.\
- Do setup at least the HTTP basic auth config. A better option would be to
  setup request signing (that's currently in progress in `teleport-webhooks`)

## Getting started

### Prerequisites

Same for all other teleport plugins:

1. A Teleport server with access to `tctl` with at least two different roles.
2. A Teleport user for the plugin to authenticate as, and their auth
   certificates.

See [access/README.md] for details on setting up the prerequisites.

### Installation

You can either build the plugin from source, or install a release tarball.

### Configuration

After initial install, you'll need to configure `teleport-webhooks`. Run
`teleport-webhooks configure > teleport-webhooks.toml` to get a template
configuration in place, and then edit it. Here's the example configuration:

```toml
# example webhooks plugin configuration TOML file
[teleport]
auth_server = "example.com:3025"                        # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/webhooks/auth.key" # Teleport GRPC client secret key
client_crt = "/var/lib/teleport/plugins/webhooks/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/webhooks/auth.cas"   # Teleport cluster CA certs

[webhook]
webhook_url = "https://mywebhook.com/ppst" # Receiver webhook URL
notify_only = false # Allow Approval / Denial actions via the Callbacks
request_states = { "Pending" = true, "Approved" = false, "Denied" = false } # What request statuses to notify about?

[http]
public_addr = "example.com" # URL on which callback server is accessible externally, e.g. [https://]teleport-proxy.example.com
# listen_addr = ":8081" # Network address in format [addr]:port on which callback server listens, e.g. 0.0.0.0:8081
https_key_file = "/var/lib/teleport/webproxy_key.pem"  # TLS private key
https_cert_file = "/var/lib/teleport/webproxy_cert.pem" # TLS certificate

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/slack.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
```

#### Notification Only mode

You can run `teleport-webhooks` in notification-only mode. In that mode, the
plugin will post updates to the provided hook URL, but will not accept any
callback HTTP request and will not change access requests status. Use this if
you need to notify another system about your requests and it already accepts
webhooks, but doesn't work with the GRPC api directly.

#### Customized request states

By changing the `request_states` map you can set which request states will
trigger the webhook, i.e. send the hook to the provided URL.

#### HTTP Basic Authentication

You can use HTTP basic auth for all incoming reqests, if the 3rd party service
provides a way to set HTTP headers on the requests.

## Zapier integration

**Zapier integration is presented here for demonstration purpose only.
Gravitational doesn't recoomend using Zapier in your approval workflows.**

### Calendar Approval workflow

With `teleport-webhooks` you can:

1. Send new access requests to Zapier
2. Zapier then creates a Google Calendar event, and invites the admin to it:
   `[Teleport {cluster_name}] {Username} requests {Roles}`. The event is created
   at the time of the request.
3. The admin can accept or deny the invitation.
4. Zapier listens to Google Calendar event updates. As soon as the event is
   accepted or denied, it sends a callback to `teleport-webhooks`.
5. `teleport-webhooks` callback server processes the request and approves or
   denies the request.

![](doc/zapier-calendar-flow.drawio.png)
