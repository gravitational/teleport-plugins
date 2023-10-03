# Teleport plugins and example applications

Teleport plugins allow you to integrate the Teleport Access Platform and Teleport workflows with other tools you use to support your infrastructure.

For example, Teleport Access Request plugins enable you to integrate access requests for resources protected by Teleport with your organization's existing messaging and project management solutions, such as Slack, JIRA, and Mattermost.
If you have a self-hosted Teleport deployment, you can find information about configuring access request plugins in [Just-in-Time Access Request Plugins](https://goteleport.com/docs/access-controls/access-request-plugins/).

## Access API

The [access](./access) package exposes a simple API for managing access requests
that can be used for writing plugins. You can find the current Teleport Access API in the main [Teleport repository](https://github.com/gravitational/teleport). For
more information, see [access/README.md](./access/README.md).

## Existing plugin guides

The Teleport documentation includes access request plugins guides for integration
with the following solutions:

- [Discord](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-discord/)
- [Email](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-email/)
- [JIRA](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-jira/)
- [Mattermost](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-mattermost/)
- [Microsoft Teams](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-msteams/)
- [PagerDuty](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-pagerduty/)
- [Slack](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-slack/)

## Event Handler

The [Teleport Event Handler Plugin](./event-handler) is used to export audit log events to a `fluentd` service. 
For more information, see [Fluentd](https://goteleport.com/docs/management/export-audit-events/fluentd/).

## Terraform Provider

The [Teleport Terraform Provider](./terraform) makes it easy to create resources using Terraform. 
For more information, see [Terraform Provider]((https://goteleport.com/docs/setup/guides/terraform-provider/).
