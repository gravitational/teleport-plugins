# Teleport Access Request MsTeams Plugin

This chart sets up and configures a Deployment for the Access Request MsTeams plugin.

## Installation

### Prerequisites

As the MsTeams setup requires to download the plugin locally to generate assets to load in MsTeams,
you must follow [the MsTeams access request guide](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-ms-teams/).
When generating the identity file, choose the "proxy" solution that generates a single file `auth.pem`.

Once the guide in finished, you should have a working `teleport-ms-teams.toml` configuration file.

Recover the following values from it:
- msapi.appID
- msapi.appSecret
- msapi.teamsAppID
- msapi.tenantID
- teleport.addr

Recover also the `auth.pem` identity file generated during the guide.

### Add the Teleport Helm repo

Run the command:
```shell
helm repo add teleport https://charts.releases.teleport.dev/
```

### Creating the identity secret

The identity file is not provided through the Helm chart, it should be already existing present in the cluster.

Run the following command to create the secret from the `auth.pem` file recovered earlier:

```shell
export NAMESPACE="your-namespace" #The namespace should already exist
kubectl create -n "$NAMESPACE" secret generic teleport-plugin-ms-teams-identity --from-file=auth_id=./auth.pem
```

### Installing the chart

Create the value file `teleport-plugin-ms-teams-values.yaml` with the following content:

```yaml
teleport:
  address: "YOUR-TELEPORT-ADDRESS"
  identitySecretName: "teleport-plugin-ms-teams-identity"

msTeams:
  appID: "YOUR-APPID"
  appSecret: "YOUR-APP-SECRET"
  tenantID: "YOUR-TENANT"
  teamsAppID: "YOUR-TEAMS-APP-ID"

roleToRecipients:
  "*": "YOUR.EMAIL@EXAMPLE.COM"
  "editor": ["YOUR.EMAIL@EXAMPLE.COM", "https://CHANNEL URL"]
```

Replace the placeholders by the values you recovered during the guide.
The `roleToRecipient` map controls which channels and users will be notified if a role is requested.

Finally, create a release from the Helm chart with the values:

```shell
helm install teleport-plugin-ms-teams teleport/teleport-plugin-ms-teams --values teleport-plugin-ms-teams-values.yaml -n "$NAMESPACE"
```
