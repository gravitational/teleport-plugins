# Teleport Slack Plugin

This package implements a simple Slack plugin using the API provided in the
[`access`](../) package which notifies about Access Requests via Slack
messages.

## Setup

[See setup instructions on Teleport's website](https://goteleport.com/teleport/docs/enterprise/workflow/ssh_approval_slack/)

You must have Go version 1.15 or higher to build.

Run `make access-slack` from the repository root to build the slack plugin. Then
you can find it in `./access/slack/build/teleport-slack`.

Detailed install steps are provided in our [docs](https://goteleport.com/docs/enterprise/workflow/ssh-approval-slack/).

## Usage

Once your Slack plugin has been configured, you can verify that it's working
correctly by using `tctl request create <user> --roles=<roles>` to simulate an
access request. If everything is working as intended, a message should appear
in the channel specified under `slack.channel`.

Select `Deny` and verify that the request was indeed denied using
`tctl request ls`.