# Teleport Access Request Email Plugin

This chart sets up and configures a Deployment for the Access Request Email plugin.

## Installation

### Prerequisites

First, you'll need to create a Teleport user and role for the plugin. The following file contains a minimal user that's needed for the plugin to work:

```yaml
---
kind: role
version: v5
metadata:
  name: teleport-plugin-email
spec:
  allow:
    logins:
    - teleport-plugin-email
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
  name: teleport-plugin-email
spec:
  roles:
    - teleport-plugin-email
```

You can either create the user and the roles by putting the YAML above into a file and issuing the following command  (you must be logged in with `tsh`):

```
tctl create user.yaml
```

or by navigating to the Teleport Web UI under `https://<yourserver>/web/users` and `https://<yourserver>/web/roles` respectively. You'll also need to create a password for the user by either clicking `Options/Reset password...` under `https://<yourserver>/web/users` on the UI or issuing `tctl users reset teleport-plugin-email` in the command line.

The next step is to create an identity file, which contains a private/public key pair and a certificate that'll identify us as the user above. To do this, log in with the newly created credentials and issue a new certificate (525600 and 8760 are both roughly a year in minutes and hours respectively):

```
tsh login --proxy=proxy.example.com --auth local --user teleport-plugin-email --ttl 525600
```

```
tctl auth sign --user teleport-plugin-email --ttl 8760h --out teleport-plugin-email-identity
```

Alternatively, you can execute the command above on one of the `auth` instances/pods.

The last step is to create the secret. The following command will create a Kubernetes secret with the name `teleport-plugin-email-identity` with the key `auth_id` in it holding the contents of the file `teleport-plugin-email-identity`:

```
kubectl create secret generic teleport-plugin-email-identity --from-file=auth_id=teleport-plugin-email-identity
```

### Installing the plugin

```
helm repo add teleport https://charts.teleport.sh/
```

```shell
helm install teleport-plugin-email teleport/teleport-plugin-email --values teleport-plugin-email-values.yaml
```

Example `teleport-plugin-email-values.yaml` for using MailGun:

```yaml
teleport:
  address: teleport.example.com:443
  identitySecretName: teleport-plugin-email-identity

mailgun:
  enabled: true
  domain: sandboxbd81caddef744a69be0e5b544ab0c3bd.mailgun.org
  privateKey: supersecretprivatekey

role_to_recipients:
  '*': access-requests@example.com
```

Alternatively, you can pass arguments from the command line (useful for one-liners or scripts):

```
helm install teleport-plugin-email teleport/teleport-plugin-email \
  --set 'teleport.address=teleport.example.com:443' \
  --set 'teleport.identitySecretName=teleport-plugin-email-identity' \
  --set 'mailgun.enabled=true' \
  --set 'mailgun.domain=sandboxbd81caddef744a69be0e5b544ab0c3b'd.mailgun.org \
  --set 'mailgun.privateKey=supersecretprivatekey' \
  --set 'roleToRecipients.*=access-requests@example.com'
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
    <td>no</td>
  </tr>

  <tr>
    <td><code>mailgun.enabled</code></td>
    <td>
      Specifies if the Mailgun integration should be enabled. Mutually exclusive with <code>smtp.enabled</code>.
      In the case of both values are set to true, <code>mailgun.enabled</code> will take precedence.
    </td>
    <td>boolean</td>
    <td><code>false</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>mailgun.domain</code></td>
    <td>Domain name of the Mailgun instance</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>mailgun.privateKey</code></td>
    <td>Private key for accessing the Mailgun instance</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
  </tr>

  <tr>
    <td><code>smtp.enabled</code></td>
    <td>
      Specifies if the MailSMTPgun integration should be enabled. Mutually exclusive with <code>mailgun.enabled</code>.
      In the case of both values are set to true, <code>mailgun.enabled</code> will take precedence.
    </td>
    <td>boolean</td>
    <td><code>false</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>smtp.host</code></td>
    <td>SMTP host.</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>smtp.port</code></td>
    <td>Port of the SMTP server.</td>
    <td>integer</td>
    <td><code>587</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>smtp.username</code></td>
    <td>Username to be used with the SMTP server.</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>smtp.password</code></td>
    <td>Password to be used with the SMTP server. Mutually exclusive with <code>smtp.passwordFile</code>.</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>smtp.passwordFile</code></td>
    <td>
      Path of the file that contains the password to be used with the SMTP server. Can be mounted via <code>volumes</code> and <code>volumeMounts</code>. Mutually exclusive with <code>smtp.password</code>.
    </td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>smtp.starttlsPolicy</code></td>
    <td>Which policy to use for secure communications: mandatory, opportunistic or disabled.</td>
    <td>string</td>
    <td><code>"mandatory"</code></td>
    <td>no</td>
  </tr>

  <tr>
    <td><code>delivery.sender</code></td>
    <td>Email address to be used in the <code>From</code> field of the emails.</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>delivery.recipients</code></td>
    <td>Array of the recipients the plugin should send emails.</td>
    <td>array</td>
    <td><code>[]</code></td>
    <td>no</td>
  </tr>

  <tr>
    <td><code>roleToRecipients</code></td>
    <td>
      Mapping of roles to a list of emails. <br />
      Example:
      <pre>
"dev" = ["developers@example.com", "user@example.com"]
"*" = ["access-requests"]</pre>
    </td>
    <td>map</td>
    <td><code>{}</code></td>
    <td>yes</td>
  </tr>

  <tr>
    <td><code>log.output</code></td>
    <td>
      Logger output. Could be <code>"stdout"</code>, <code>"stderr"</code> or a file name,
      eg. <code>"/var/lib/teleport/email.log"</code>
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
