# PROJECT MOVED

The `teleport-plugins` repository has been merged into
[the `gravitational/teleport` repository](https://github.com/gravitational/teleport)
and is now a read-only archive.
Plugin development continues in the main Teleport repository.

This change allows us to:
- backport plugin changes for previous majors. Starting with v15, plugins will
  receive bugfixes even after a new major is out.
- reuse existing CI/CD pipelines and build artifacts for more CPU
  architectures. Starting with v16, we will offer builds for amd64 and arm64 cpu 
  architectures. Container images will also have a new `-debug` variant with
  Busybox to open a shell and troubleshoot issues.
- ensure there is no delay between the Teleport and the plugin releases.
- generate documentation automatically for the Terraform provider and Helm charts.
- benefit from automated dependency scans and updates in the main teleport repository.

You can now find the teleport-plugins code in the teleport repository:
- the Terraform provider now lives in [`integrations/terraform`](https://github.com/gravitational/teleport/tree/master/integrations/terraform)
- the event-handler now lives in [`integrations/event-handler`](https://github.com/gravitational/teleport/tree/master/integrations/event-handler)
- the access plugins (Slack, Discord, Jira, Pagerduty, ...) now live in [`integrations/access`](https://github.com/gravitational/teleport/tree/master/integrations/access)
- the event-handler Helm chart now lives in [`examples/chart/event-handler`](https://github.com/gravitational/teleport/tree/master/examples/chart/event-handler)
- the access plugins charts now live in [`examples/chart/access`](https://github.com/gravitational/teleport/tree/master/examples/chart/access)
- the release go tools have been moved [in the private `teleport.e` repository](https://github.com/gravitational/teleport.e/tree/master/tooling/plugins) which is running the release pipeline
- the content of `docker/` and `apps/` is seemingly unmaintained and was not migrated

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
