# Teleport Access Request Jira Plugin

This chart sets up and configures a Deployment for the Access Request Jira plugin.

## Installation

### Prerequisites

First, you'll need to create a Teleport user and role for the plugin. The following file contains a minimal user that's needed for the plugin to work:

```yaml
---
kind: role
version: v6
metadata:
  name: teleport-plugin-jira
spec:
  allow:
    logins:
    - teleport-plugin-jira
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
  name: teleport-plugin-jira
spec:
  roles:
    - teleport-plugin-jira
```

You can either create the user and the roles by putting the YAML above into a file and issuing the following command  (you must be logged in with `tsh`):

```
tctl create user.yaml
```

or by navigating to the Teleport Web UI under `https://<yourserver>/web/users` and `https://<yourserver>/web/roles` respectively. You'll also need to create a password for the user by either clicking `Options/Reset password...` under `https://<yourserver>/web/users` on the UI or issuing `tctl users reset teleport-plugin-jira` in the command line.

The next step is to create an identity file, which contains a private/public key pair and a certificate that'll identify us as the user above. To do this, log in with the newly created credentials and issue a new certificate (525600 and 8760 are both roughly a year in minutes and hours respectively):

```
tsh login --proxy=teleport.example.com --auth local --user teleport-plugin-jira --ttl 525600
```

```
tctl auth sign --user teleport-plugin-jira --ttl 8760h --out teleport-plugin-jira-identity
```

Alternatively, you can execute the command above on one of the `auth` instances/pods.

The last step is to create the secret. The following command will create a Kubernetes secret with the name `teleport-plugin-jira-identity` with the key `auth_id` in it holding the contents of the file `teleport-plugin-jira-identity`:

```
kubectl create secret generic teleport-plugin-jira-identity --from-file=auth_id=teleport-plugin-jira-identity
```

### Attaching the certificate

You'll need both a certificate and it's private key to secure the WebHook connections coming from Jira Server or Jira Cloud. Once you have them, create a Kubernetes secret similar to the one below:

```yaml
apiVersion: v1
kind: Secret
type: kubernetes.io/tls
metadata:
  name: teleport-plugin-jira-tls
data:
  tls.crt: LS0...
  tls.key: LS0...
```

Make sure you apply base64 on the value (or use Kubernetes Secret's `stringData` field instead of `data`).

### Installing the plugin

```
helm repo add teleport https://charts.releases.teleport.dev/
```

```shell
helm install teleport-plugin-jira teleport/teleport-plugin-jira --values teleport-plugin-jira-values.yaml
```

Example `teleport-plugin-jira-values.yaml`:

```yaml
teleport:
  address: teleport.example.com:443
  identitySecretName: teleport-plugin-jira-identity

jira:
  url: "https://jira.example.net"
  username: "user@example.com"
  apiToken: "exampleapitoken"
  project: "REQS"
  issueType: "Task"

http:
  publicAddress: "teleport-plugin-jira.example.com"
  tlsFromSecret: "teleport-plugin-jira-tls"
  # Uncomment and change the following lines if your secret is structured
  # differently then the example above
  # tlsKeySecretPath: "tls.key"
  # tlsCertSecretPath: "tls.crt"

  basicAuth:
    user: "basicauthuser"
    password: "basicauthpassword"

# Uncomment the following line on AWS
# chartMode: "aws"
```

Make sure you protect the endpoint by setting a strong basic auth password in the `http` section!

See [Settings](#settings) for more details.

### Set up the Jira project

[Follow these instructions](https://goteleport.com/docs/enterprise/workflow/ssh-approval-jira-cloud/#setting-up-your-jira-project) to set up a Jira project for the incoming access requests.

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
    <td><code>chartMode</code></td>
    <td>
      When set to <code>"aws"</code>, it'll add the proper annotations to the created service
      to ensure the AWS LoadBalancer is set up properly. Additional annotations can be added
      using <code>serviceAnnotations</code>.
    </td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
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
    <td><code>jira.url</code></td>
    <td>URL of the Jira server</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>jira.username</code></td>
    <td>Username of the bot user in Jira to use for creating issues.</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>jira.apiToken</code></td>
    <td>API token of the bot user.</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>jira.project</code></td>
    <td>Short code of the project in Jira in which issues will be created</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>jira.issueType</code></td>
    <td>Type of the issues to be created on access requests (eg. Bug, Task)</td>
    <td>string</td>
    <td><code>"Task"</code></td>
    <td>no</td>
  </tr>

  <tr>
    <td><code>http.publicAddress</code></td>
    <td>The domain name which will be assigned to the service</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>http.tlsFromSecret</code></td>
    <td>Name of the Kubernetes secret where the TLS key and certificate will be mounted</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>http.tlsKeySecretPath</code></td>
    <td>Path of the TLS key in the secret specified by <code>http.tlsFromSecret</code></td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>http.tlsCertSecretPath</code></td>
    <td>Path of the TLS certificate in the secret specified by <code>http.tlsFromSecret</code></td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
  </tr>

  <tr>
    <td><code>http.basicAuth.username</code></td>
    <td>Username for the basic authentication. The plugin will require a m atching `Authorization` header in case both the username and the password are specified.</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>http.basicAuth.password</code></td>
    <td>Password for the basic authentication. The plugin will require a m atching `Authorization` header in case both the username and the password are specified.</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
  </tr>

  <tr>
    <td><code>log.output</code></td>
    <td>
      Logger output. Could be <code>"stdout"</code>, <code>"stderr"</code> or a file name,
      eg. <code>"/var/lib/teleport/jira.log"</code>
    </td>
    <td>string</td>
    <td><code>"stdout"</code></td>
  </tr>
  <tr>
    <td><code>log.severity</code></td>
    <td>
      Logger severity. Possible values are <code>"INFO"</code>, <code>"ERROR"</code>,
      <code>"DEBUG"</code> or <code>"WARN"</code>.
    </td>
    <td>string</td>
    <td><code>"INFO"</code></td>
  </tr>
</table>
