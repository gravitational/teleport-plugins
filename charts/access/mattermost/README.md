# Teleport Access Request Mattermost Plugin

This chart sets up and configures a Deployment for the Access Request Mattermost plugin.

## Installation

### Prerequisites

First, you'll need to create a Teleport user and role for the plugin. The following file contains a minimal user that's needed for the plugin to work:

```yaml
---
kind: role
version: v5
metadata:
  name: teleport-plugin-mattermost
spec:
  allow:
    logins:
    - teleport-plugin-mattermost
    rules:
    - resources:
      - access_request
      verbs:
      - list
      - read
      - update
  options:
    forward_agent: false
    max_session_ttl: 8760h0m0s
    port_forwarding: false
---
kind: user
version: v2
metadata:
  name: teleport-plugin-mattermost
spec:
  roles:
    - teleport-plugin-mattermost
```

You can either create the user and the roles by putting the YAML above into a file and issuing the following command  (you must be logged in with `tsh`):

```console
tctl create user.yaml
```

or by navigating to the Teleport Web UI under `https://<yourserver>/web/users` and `https://<yourserver>/web/roles` respectively. You'll also need to create a password for the user by either clicking `Options/Reset password...` under `https://<yourserver>/web/users` on the UI or issuing `tctl users reset teleport-plugin-mattermost` in the command line.

The next step is to create an identity file, which contains a private/public key pair and a certificate that'll identify us as the user above. To do this, log in with the newly created credentials and issue a new certificate (525600 and 8760 are both roughly a year in minutes and hours respectively):

```console
tsh login --proxy proxy.example.com --auth local --user teleport-plugin-mattermost --ttl 525600
```

```console
tctl auth sign --user teleport-plugin-mattermost --ttl 8760h --out teleport-plugin-mattermost-identity
```

Alternatively, you can execute the command above on one of the `auth` instances/pods.

The last step is to create the secret. The following command will create a Kubernetes secret with the name `teleport-plugin-mattermost-identity` with the key `auth_id` in it holding the contents of the file `teleport-plugin-mattermost-identity`:

```console
kubectl create secret generic teleport-plugin-mattermost-identity --from-file=auth_id=teleport-plugin-mattermost-identity
```

### Installing the plugin

```console
helm repo add teleport https://charts.releases.teleport.dev/
```

```console
helm install teleport-plugin-mattermost teleport/teleport-plugin-mattermost --values teleport-plugin-mattermost-values.yaml
```

Example `teleport-plugin-mattermost-values.yaml`:

```yaml
teleport:
  address: teleport.example.com:443
  identitySecretName: teleport-plugin-mattermost-identity

mattermost:
  url: https://mattermost.example.com/
  token: mattermosttoken
  recipients: [access-requests@example.com, "#example-channel"]
```

See [Settings](#settings) for more details.

## Settings

The following values can be set for the Helm chart:

<table>
  <tr>
    <th>Name</th>
    <th>Description</th>
    <th>Type</th>
    <th>Default</th>
    <th>Required</th>
  </tr>

  <tr>
    <td><code>teleport.address</code></td>
    <td>Host/port combination of the teleport auth server</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>teleport.identitySecretName</code></td>
    <td>Name of the Kubernetes secret that contains the credentials for the connection</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>teleport.identitySecretPath</code></td>
    <td>Key of the field in the secret specified by <code>teleport.identitySecretName</code></td>
    <td>string</td>
    <td><code>"auth_id"</code></td>
    <td>yes</td>
  </tr>

  <tr>
    <td><code>mattermost.url</code></td>
    <td>URL of the Mattermost server</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>mattermost.token</code></td>
    <td>Token to be used to authenticate with Mattermost</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>mattermost.recipients</code></td>
    <td>Array of the recipients the plugin should send access requests to.</td>
    <td>array</td>
    <td><code>[]</code></td>
    <td>yes</td>
  </tr>

  <tr>
    <td><code>log.output</code></td>
    <td>
      Logger output. Could be <code>"stdout"</code>, <code>"stderr"</code> or a file name,
      eg. <code>"/var/lib/teleport/mattermost.log"</code>
    </td>
    <td>string</td>
    <td><code>"stdout"</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>log.severity</code></td>
    <td>
      Logger severity. Possible values are <code>"INFO"</code>, <code>"ERROR"</code>,
      <code>"DEBUG"</code> or <code>"WARN"</code>.
    </td>
    <td>string</td>
    <td><code>"INFO"</code></td>
    <td>no</td>
  </tr>
</table>
