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

A basic slack plugin (WIP) can be found in [access/slack](./access/slack). The
plugin can be built with `make access-slack` and instructions for configuring
the plugin can be found in the plugin's [README](./access/slack/README.md).

### JIRA Bot

A basic Teleport / JIRA integration (WIP) can be found in
[access/jira](./access/jira). The plugin can be built with `make access-jira`
and instructions for configuring the plugin can be found in the plugin's
[README](./access/jira/README.md).

### Mattermost Bot

Mattermost is a private cloud messaging platform (think Slack for enterprise).
Teleport provides a Mattermost integration that supports request flows similar
to Slack integration above. The plugin can be built with
`make access-mattermost`, and instructions for configuring the plugin can be
found in the plugin's [README](./access/mattermost/README.md).

### Pagerduty Extension

A Teleport integration with Pagerduty that allows your team to treat Teleport
permission requests as Pagerduty incidents, and provides Pagerduty special
actions to approve or deny permission requests. Run `make teleport-pagerduty` to
build it. More docs in the [README](./access/pagerduty/README.md).

## Notes

Use `scripts/dep` for dependencies management. It is a wrapper over `dep` which
ignores git submodules.
