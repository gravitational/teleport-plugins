# Teleport / Gitlab plugin

The plugin allows teams to setup permissions workflow over their existing or new Gitlab projects. 
When someone requests new permissions, an issue will be opened, and the team members can assign approval or denied label to the issue to approve or deny the request.


## Quick setup

To get things up & running quickly:


1. On Gitlab, go "User Settings" -> "Access Tokens". Create a token with api scope, remember the token.
2. Create a project, get its numeric "Project ID" from "Project Overview" -> "Details" page.
3. You might want to create the Board with lists: `Teleport: Pending`, `Teleport: Approved`, and `Teleport: Denied`. The plugin will work if you just change labels on issues, but with a Board you can just drag the issue into a status-column you want.
4. Create an /etc/teleport-gitlab.yml

```toml
# /etc/teleport-gitlab.toml
[teleport]
auth_server = "example.com:3025"                         # Teleport Auth Server GRPC API address
client_key = "/var/lib/teleport/plugins/gitlab/auth.key" # Teleport GRPC client secret key
client_crt = "/var/lib/teleport/plugins/gitlab/auth.crt" # Teleport GRPC client certificate
root_cas = "/var/lib/teleport/plugins/gitlab/auth.cas"   # Teleport cluster CA certs

[db]
path = "/var/lib/teleport/plugins/gitlab/database" # Path to the database file

[gitlab]
url = ""                                   # Leave empty if you are using cloud
token = "token"                            # GitLab API Token
project_id = "1812345"                     # GitLab Project ID
webhook_secret = "your webhook passphrase" # A secret used to encrypt data we use in webhooks. Basically anything you'd like.

[http]
public_addr = "example.com" # URL on which webhook server is accessible externally, e.g. [https://]teleport-gitlab.example.com
# listen_addr = ":8081" # Network address in format [addr]:port on which webhook server listens, e.g. 0.0.0.0:443
https_key_file = "/var/lib/teleport/plugins/gitlab/server.key"  # TLS private key
https_cert_file = "/var/lib/teleport/plugins/gitlab/server.crt" # TLS certificate

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/gitlab.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
```

The plugin creates labels on Gitlab automatically if they don't exist yet. You don't have to set anything up on Gitlab, except for the project (create new, or grab project ID from an existing one), and the Board.
