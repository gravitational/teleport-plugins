# Teleport JIRA Bot

This package provides Teleport <-> Jira integration that allows teams to approve or deny Access Requests on a Jira Project Board.
It works with both Jira Cloud and Jira Server 8.

## Setup

Setup process is different for the Jira Cloud and Jira Server editions:

- Please refer to [INSTALL-JIRA-CLOUD.md](./INSTALL-JIRA-CLOUD.md) for a detailed Jira Cloud getting started guide.
- Jira Server getting started guide: [INSTALL-JIRA-SERVER.md](./INSTALL-JIRA-SERVER.md)

Next few paragraphs will guide you through building the plugin locally.

```bash

# Check out the repo
git clone git@github.com:gravitational/teleport-plugins.git
cd teleport-plugins

# Build the bot
make access-jirabot

# Configure the plugin
./build/access-jirabot configure > teleport-jirabot.toml

# Run the plugin, assuming you have teleport running: 
./build/access-jirabot start
```

### Example config file

```toml
[teleport]
auth-server = "example.com:3025"  # Auth GRPC API address
client-key = "/var/lib/teleport/plugins/jirabot/auth.key" # Teleport GRPC client secret key
client-crt = "/var/lib/teleport/plugins/jirabot/auth.crt" # Teleport GRPC client certificate
root-cas = "/var/lib/teleport/plugins/jirabot/auth.cas"   # Teleport cluster CA certs

[jira]
url = "https://[my-jira].atlassian.net"    # JIRA URL. For JIRA Cloud, https://[my-jira].atlassian.net
username = "bot@example.com"        # JIRA username
api-token = "token"                 # JIRA API Basic Auth token, or our password in case you're using Jira Server.
project = "MYPROJ"                  # JIRA Project key

[http]
listen = ":8081"          # JIRA webhook listener
# host = "example.com"    # Host name by which bot is accessible
# https-key-file = "/var/lib/teleport/plugins/jirabot/server.key"  # TLS private key
# https-cert-file = "/var/lib/teleport/plugins/jirabot/server.crt" # TLS certificate

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/jirabot.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
```

### `[teleport]`

This configuration section ensures that the bot can talk to your teleport
auth server & manage access-requests.  Use `tctl auth sign --format=tls`
to generate the required PEM files, and make sure that the Auth Server's
GRPC API is accessible at the address indicated by `auth-server`.

*NOTE*: The slackbot must be given a teleport user identity with
appropriate permissions.  See the [access package README](../README.md#authentication)
for an example of how to configure an appropriate user & role.

### `[jira]`

This block manages interacting with your Jira installation. You'd need to paste your Jira dashboard URL, project key, and your access token.
You can grab a JIRA Cloud API token [on this page](https://id.atlassian.com/manage/api-tokens).

You'll need to setup a custom issue field on Jira. It's name must be `TeleportAccessRequestId`, and it should be on the issue type `Task` in the project you intend to use with Teleport.GET 

### `[http]`

Jirabot starts it's own http server and listens to a webhook from JIRA, this block covers the http server's behavior, including the listen host & port, and TLS certs.

## Usage

Once your slackbot has been configured, you can varify that it is working
correctly by using `tctl request create <user> --roles=<roles>` to simulate
an access request. You should see a new JIRA card pop up. You can now drag the card to either Approved or Denied column, and that should approve or deny the request on Teleport. You can verify that the request was indeed processed correctly by running `tctl request ls`.


## Security

Currently, this Bot does not make any distinction about *who* approves/denies
a request. Any user with access to the Jira project, if not constrained by JIRA workflows, can approve or deny Teleport requests. You can use JIRA workflows to limit who can approve or deny the requests. 
