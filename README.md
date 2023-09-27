# Teleport Access Request plugins and example applications

Access Request plugins enable you to integrate access requests for resources protected by
Teleport with your organization's existing messaging and project management solutions, such as Slack, JIRA, and Mattermost.
If you have a self-hosted Teleport deployment, you can find information for configuring access
request plugins in [Just-in-Time Access Request Plugins](https://goteleport.com/docs/access-controls/access-request-plugins/).

[![Build Status](https://drone.platform.teleport.sh/api/badges/gravitational/teleport-plugins/status.svg)](https://drone.platform.teleport.sh/gravitational/teleport-plugins/)

## Access API

The [access](./access) package exposes a simple API for managing access requests
which can be used for writing plugins. More info can be found in
[access/README.md](./access/README.md), including instructions on how to
properly provision necessary certificates.

The [access/example](./access/example) plugin automatically approves access
requests based on a user whitelist. This is a good place to start if you are
trying to understand how to use the [`access`](./access) API.

Use `make access-example` to build the plugin and
`./build/access-example configure` to print out a sample configuration file.

## Existing plugin guides

The Teleport documentation includes access request plugins guides for integration
with the following solutions:

- [Discord](https://goteleport.com/docs/ver/15.x/access-controls/access-request-plugins/ssh-approval-discord/)
- [Email](https://goteleport.com/docs/ver/15.x/access-controls/access-request-plugins/ssh-approval-email/)
- [JIRA](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-jira/)
- [Mattermost](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-mattermost/)
- [Microsoft Teams](https://goteleport.com/docs/ver/15.x/access-controls/access-request-plugins/ssh-approval-msteams/)
- [PagerDuty](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-pagerduty/)
- [Slack](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-slack/)

## Event Handler

The [Teleport Event Handler Plugin](./event-handler) is used to export audit log events to a `fluentd` service. 
For more information, see [Fluentd](https://goteleport.com/docs/management/export-audit-events/fluentd/).

## Terraform Provider

The [Teleport Terraform Provider](./terraform) makes it easy to create resources using Terraform. 
For more information, see [Terraform Provider]((https://goteleport.com/docs/setup/guides/terraform-provider/).
