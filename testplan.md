
## Manual Testing Plan

Below are the items that should be manually tested with each release of Teleport Plugins.
These tests should be run on both a fresh install of the version to be released
as well as an upgrade of the previous version of Teleport Plugin.

This test plan is based on [Teleports](https://github.com/gravitational/teleport/blob/master/docs/testplan.md), 
it should be ran on plugin release and with each new Teleport Release. 

### General Plugin Setup

- [ ] Able to export auth creds from Teleport. `$ tctl auth sign --format=tls --user=access-plugin --out=auth --ttl=8760h`
- [ ] Able to create a user and role for access.

### Slack Plugin

- [ ] Creating Slack Plugin and OAuth token instructions are up-to-date. 
- [ ] `teleport-slackbot configure` outputs valid TOML
- [ ] Plugin started with TLS
- [ ] Plugin started --insecure-no-tls

- [ ] End users `tsh login --request-roles=REQUESTED_ROLE` appears in Slack
- [ ] Any Slack users in specific room is able to Approve the request.
- [ ] End user now sees role approved in CLI
- [ ] Any Slack users in specific room is able to Deny the request.
- [ ] End user now sees role denied in CLI

- [ ] A long running request should gracefully degrade

- [ ] Teleport Audit log displays correct user approver/deny in UI ( /audit/events )

### Mattermost Plugin

- [ ] Creating Mattermost Plugin and OAuth token instructions are up-to-date. 
- [ ] `teleport-mattermost configure` outputs valid TOML
- [ ] Plugin started with TLS
- [ ] Plugin started --insecure-no-tls

- [ ] End users `tsh login --request-roles=REQUESTED_ROLE` appears in Mattermost
- [ ] Any Mattermost users in specific room is able to Approve the request.
- [ ] End user now sees role approved in CLI
- [ ] Any Mattermost users in specific room is able to Deny the request.
- [ ] End user now sees role denied in CLI

- [ ] A long running request should gracefully degrade

- [ ] Teleport Audit log displays correct user approver/deny in UI ( /audit/events )

### Pagerduty Plugin

- [ ] Creating PagerDuty Plugin and OAuth token instructions are up-to-date. 
- [ ] `teleport-pagerduty configure` outputs valid TOML
- [ ] Plugin started with TLS
- [ ] Plugin started --insecure-no-tls

- [ ] End users `tsh login --request-roles=REQUESTED_ROLE` appears in PagerDuty
- [ ] Any PagerDuty on call is able to Approve the request.
- [ ] End user now sees role approved in CLI
- [ ] Any PagerDuty on call  in specific room is able to Deny the request.
- [ ] End user now sees role denied in CLI

- [ ] A long running request should gracefully degrade

- [ ] Teleport Audit log displays correct user approver/deny in UI ( /audit/events )


### Jira Cloud Plugin

- [ ] Setting up Jira Board and OAuth token instructions are up-to-date. 
- [ ] `teleport-jirabot configure` outputs valid TOML
- [ ] Plugin started with TLS
- [ ] Plugin started --insecure-no-tls

- [ ] End users `tsh login --request-roles=REQUESTED_ROLE` appears in Jira Board
- [ ] Any Jira board member is able to Approve the request.
- [ ] End user now sees role approved in CLI
- [ ] Any Jira board member is able to Deny the request.
- [ ] End user now sees role denied in CLI

- [ ] A long running request should gracefully degrade

- [ ] Teleport Audit log displays correct user approver/deny in UI ( /audit/events )

### Jira Server Plugin

- [ ] Setup has been configured using Jira Server 8+
- [ ] Setting up Jira Board and OAuth token instructions are up-to-date. 
- [ ] `teleport-jirabot configure` outputs valid TOML
- [ ] Plugin started with TLS
- [ ] Plugin started --insecure-no-tls

- [ ] End users `tsh login --request-roles=REQUESTED_ROLE` appears in Jira Board
- [ ] Any Jira board member is able to Approve the request.
- [ ] End user now sees role approved in CLI
- [ ] Any Jira board member is able to Deny the request.
- [ ] End user now sees role denied in CLI

- [ ] A long running request should gracefully degrade

- [ ] Teleport Audit log displays correct user approver/deny in UI ( /audit/events )

### Gitlab Plugin

- [ ] Setup has been configured using [Gitlab](https://about.gitlab.com/install/)
- [ ] Setting up Github Board and OAuth token instructions are up-to-date. 
- [ ] `teleport-gitlab configure` outputs valid TOML
- [ ] Plugin started with TLS
- [ ] Plugin started --insecure-no-tls

- [ ] End users `tsh login --request-roles=REQUESTED_ROLE` appears in Gitlab Board
- [ ] Any Gitlab board member is able to Approve the request.
- [ ] End user now sees role approved in CLI
- [ ] Any Gitlab board member is able to Deny the request.
- [ ] End user now sees role denied in CLI

- [ ] A long running request should gracefully degrade

- [ ] Teleport Audit log displays correct user approver/deny in UI ( /audit/events )