# Teleport Access Request PagerDuty Plugin

This chart sets up and configures a Deployment for the Access Request PagerDuty plugin.

## Installation

### Prerequisites

First, you'll need to create a Teleport user and role for the plugin. The following file contains a minimal user that's needed for the plugin to work:

```yaml
---
kind: role
version: v5
metadata:
  name: teleport-plugin-pagerduty
spec:
  allow:
    logins:
    - teleport-plugin-pagerduty
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
  name: teleport-plugin-pagerduty
spec:
  roles:
    - teleport-plugin-pagerduty
```

You can either create the user and the roles by putting the YAML above into a file and issuing the following command  (you must be logged in with `tsh`):

```console
tctl create user.yaml
```

or by navigating to the Teleport Web UI under `https://<yourserver>/web/users` and `https://<yourserver>/web/roles` respectively. You'll also need to create a password for the user by either clicking `Options/Reset password...` under `https://<yourserver>/web/users` on the UI or issuing `tctl users reset teleport-plugin-pagerduty` in the command line.

The next step is to create an identity file, which contains a private/public key pair and a certificate that'll identify us as the user above. To do this, log in with the newly created credentials and issue a new certificate (525600 and 8760 are both roughly a year in minutes and hours respectively):

```console
tsh login --proxy proxy.example.com --auth local --user teleport-plugin-pagerduty --ttl 525600
```

```console
tctl auth sign --user teleport-plugin-pagerduty --ttl 8760h --out teleport-plugin-pagerduty-identity
```

Alternatively, you can execute the command above on one of the `auth` instances/pods.

The last step is to create the secret. The following command will create a Kubernetes secret with the name `teleport-plugin-pagerduty-identity` with the key `auth_id` in it holding the contents of the file `teleport-plugin-pagerduty-identity`:

```console
kubectl create secret generic teleport-plugin-pagerduty-identity --from-file=auth_id=teleport-plugin-pagerduty-identity
```

### Installing the plugin

```console
helm repo add teleport https://charts.releases.teleport.dev/
```

```console
helm install teleport-plugin-pagerduty teleport/teleport-plugin-pagerduty --values teleport-plugin-pagerduty-values.yaml
```

Example `teleport-plugin-pagerduty-values.yaml`:

```yaml
teleport:
  address: teleport.example.com:443
  identitySecretName: teleport-plugin-pagerduty-identity

pagerduty:
  apiKey: pagerdutyapikey
  userEmail: pagerduty-bot-user@example.com
  notifyService: "request approvals"
  servies:
    - on-call
    - support
```

See [Settings](#settings) for more details.

### Setting up roles

After the PagerDuty plugin has been set up correctly, you'll need to adjust the roles you'd like to set up with it by adding the proper annotations based on your use case. For more information, visit [Setting up Pagerduty notification alerts](../../../access/pagerduty/README.md#setting-up-pagerduty-notification-alerts) in the plugin's documentation.

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
    <td><code>pagerduty.apiKey</code></td>
    <td>PagerDuty API Key</td>
    <td>string</td>
    <td><code></code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>pagerduty.userEmail</code></td>
    <td>PagerDuty bot user email</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>

  <tr>
    <td><code>log.output</code></td>
    <td>
      Logger output. Could be <code>"stdout"</code>, <code>"stderr"</code> or a file name,
      eg. <code>"/var/lib/teleport/pagerduty.log"</code>
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
