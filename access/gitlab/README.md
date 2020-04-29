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
auth-server = "localhost:3025"  # Auth GRPC API address
client-key = "/var/lib/teleport/plugins/gitlab/auth.key"
client-crt = "/var/lib/teleport/plugins/gitlab/auth.crt"
root-cas = "/var/lib/teleport/plugins/gitlab/auth.cas"

[gitlab]
token = "<API_TOKEN>"
project-id = "<PROJECT_ID>"
webhook-secret = "something-secret"

[http]
listen = ":8081"
base-url = "https://your-server.example.com/teleport-gitlab"
```

The plugin creates labels on Gitlab automatically if they don't exist yet. You don't have to set anything up on Gitlab, except for the project (create new, or grab project ID from an existing one), and the Board.
