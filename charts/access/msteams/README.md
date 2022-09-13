# Teleport Access Request MsTeams Plugin

This chart sets up and configures a Deployment for the Access Request MsTeams plugin.

## Installation

### Prerequisites

As the MsTeams setup requires to download the plugin locally to generate assets to load in MsTeams,
you must follow [the MsTeams access request guide](https://goteleport.com/docs/access-controls/access-request-plugins/ssh-approval-msteams/).
When generating the identity file, choose the "Connect to the Proxy Service" tab that generates a single file `auth.pem`.

Once the guide in finished, you should have a working `teleport-msteams.toml` configuration file.

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
kubectl create -n "$NAMESPACE" secret generic teleport-plugin-msteams-identity --from-file=auth_id=./auth.pem
```

### Installing the chart

Create the value file `teleport-plugin-msteams-values.yaml` with the following content:

```yaml
teleport:
  address: "YOUR-TELEPORT-ADDRESS"
  identitySecretName: "teleport-plugin-msteams-identity"

msTeams:
  appID: "YOUR-APPID"
  appSecret: "YOUR-APP-SECRET"
  tenantID: "YOUR-TENANT"
  teamsAppID: "YOUR-TEAMS-APP-ID"

roleToRecipients:
  "*": "YOUR.EMAIL@EXAMPLE.COM"
  "editor": ["YOUR.EMAIL@EXAMPLE.COM", "https://CHANNEL URL"]
```

_Note: If you prefer to keep `appSecret` off your values you can put it in a Kubernetes secret and specify the secret
name and secret key with the values `msTeams.appSecretFromSecret` and `msTeams.appSecretFromSecretKey`._

Replace the placeholders by the values you recovered during the guide.
The `roleToRecipient` map controls which channels and users will be notified if a role is requested.

Finally, create a release from the Helm chart with the values:

```shell
helm install teleport-plugin-msteams teleport/teleport-plugin-msteams --values teleport-plugin-msteams-values.yaml -n "$NAMESPACE"
```
