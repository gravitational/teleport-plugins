# teleport-plugins

Set of plugins for Teleport

[![Build Status](https://drone.gravitational.io/api/badges/gravitational/teleport-plugins/status.svg)](https://drone.gravitational.io/gravitational/teleport-plugins)

## Access API

The [access](./access) package exposes a simple API for managing access requests
which can be used for writing plugins. More info can be found in
[access/README.md](./access/README.md), including instructions on how to
properly provision necessary certificates.

### Example

The [access/example](./access/example) plugin automatically approves access
requests based on a user whitelist. This is a good place to start if you are
trying to understand how to use the [`access`](./access) API.

Use `make access-example` to build the plugin and
`./build/access-example configure` to print out a sample configuration file.

### Slack Bot

[See setup instructions on Teleport's website](https://goteleport.com/teleport/docs/enterprise/workflow/ssh_approval_slack/)

A basic slack plugin (WIP) can be found in [access/slack](./access/slack). The
plugin can be built with `make access-slack` and instructions for configuring
the plugin can be found in the plugin's [README](./access/slack/README.md).

### JIRA Bot

- [See detailed setup instructions for Jira Cloud on the website](https://goteleport.com/teleport/docs/enterprise/workflow/ssh_approval_jira_cloud/)
- [See detailed setup instructions for Jira Server on the website](https://goteleport.com/teleport/docs/enterprise/workflow/ssh_approval_jira_server/)

A basic Teleport / JIRA integration (WIP) can be found in
[access/jira](./access/jira). The plugin can be built with `make access-jira`
and instructions for configuring the plugin can be found in the plugin's
[README](./access/jira/README.md).

### Mattermost Bot

[See setup instructions on Teleport's website](https://goteleport.com/teleport/docs/enterprise/workflow/ssh_approval_mattermost/)

Mattermost is a private cloud messaging platform (think Slack for enterprise).
Teleport provides a Mattermost integration that supports request flows similar
to Slack integration above. The plugin can be built with
`make access-mattermost`, and instructions for configuring the plugin can be
found in the plugin's [README](./access/mattermost/README.md).

### Pagerduty Extension

[See setup instructions on Teleport's website](https://goteleport.com/teleport/docs/enterprise/workflow/ssh_approval_pagerduty/)

A Teleport integration with Pagerduty that allows your team to treat Teleport
permission requests as Pagerduty incidents, and provides Pagerduty special
actions to approve or deny permission requests. Run `make teleport-pagerduty` to
build it. More docs in the [README](./access/pagerduty/README.md).

### Webhooks

`teleport-webhooks` provides webhooks compatibility for Teleport. It allows
sendind webhooks when a new request is created, or a request state is changed,
and it allows optionally listening for the 3rd party app callbacks to facilitate
the approval workdlow. See more in the [access/webhooks/README.md](/access/webhooks/README.md)

## Notes

Use `scripts/dep` for dependencies management. It is a wrapper over `dep` which
ignores git submodules.
