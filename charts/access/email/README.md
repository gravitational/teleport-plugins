# Teleport Access Request Email Plugin

This chart sets up and configures a Deployment for the Access Request Email plugin.

## Settings

The following values can be set for the Helm chart:

<table>
  <tr>
    <th>Setting</th>
    <th>Type</th>
    <th>Default value</th>
    <th>Description</th>
  </tr>

  <tr>
    <td><code>teleport.address</code></td>
    <td>string</td>
    <td><code>"auth.example.com:3025"</code></td>
    <td>Host/port combination of the teleport auth server</td>
  </tr>
  <tr>
    <td><code>teleport.identitySecretName</code></td>
    <td>string</td>
    <td><code>""</code></td>
    <td>Name of the Kubernetes secret that contains the credentials for the connection</td>
  </tr>
  <tr>
    <td><code>teleport.identitySecretPath</code></td>
    <td>string</td>
    <td><code>""</code></td>
    <td>Key of the field in the secret specified by <code>teleport.identitySecretName</code></td>
  </tr>

  <tr>
    <td><code>mailgun.enabled</code></td>
    <td>boolean</td>
    <td><code>false</code></td>
    <td>
      Specifies if the Mailgun integration should be enabled. Mutually exclusive with <code>smtp.enabled</code>.
      In the case of both values are set to true, <code>mailgun.enabled</code> will take precedence.
    </td>
  </tr>
  <tr>
    <td><code>mailgun.domain</code></td>
    <td>string</td>
    <td><code>"sandboxbd81caddef744a69be0e5b544ab0c3bd.mailgun.org"</code></td>
    <td>Domain name of the Mailgun instance</td>
  </tr>
  <tr>
    <td><code>mailgun.privateKey</code></td>
    <td>string</td>
    <td><code>""</code></td>
    <td>Private key for accessing the Mailgun instance</td>
  </tr>

  <tr>
    <td><code>smtp.enabled</code></td>
    <td>boolean</td>
    <td><code>false</code></td>
    <td>
      Specifies if the MailSMTPgun integration should be enabled. Mutually exclusive with <code>mailgun.enabled</code>.
      In the case of both values are set to true, <code>mailgun.enabled</code> will take precedence.
    </td>
  </tr>
  <tr>
    <td><code>smtp.host</code></td>
    <td>string</td>
    <td><code>"smtp.example.com"</code></td>
    <td>SMTP host.</td>
  </tr>
  <tr>
    <td><code>smtp.port</code></td>
    <td>integer</td>
    <td><code>587</code></td>
    <td>Port of the SMTP server.</td>
  </tr>
  <tr>
    <td><code>smtp.username</code></td>
    <td>string</td>
    <td><code>"username@example.com"</code></td>
    <td>Username to be used with the SMTP server.</td>
  </tr>
  <tr>
    <td><code>smtp.password</code></td>
    <td>string</td>
    <td><code>""</code></td>
    <td>Password to be used with the SMTP server. Mutually exclusive with <code>smtp.passwordFile</code>.</td>
  </tr>
  <tr>
    <td><code>smtp.passwordFile</code></td>
    <td>string</td>
    <td><code>""</code></td>
    <td>Path of the file that contains the password to be used with the SMTP server. Can be mounted via <code>volumes</code> and <code>volumeMounts</code>. Mutually exclusive with <code>smtp.password</code>.</td>
  </tr>

  <tr>
    <td><code>delivery.sender</code></td>
    <td>string</td>
    <td><code>noreply@example.com</code></td>
    <td>Email address to be used in the <code>From</code> field of the emails.</td>
  </tr>
  <tr>
    <td><code>delivery.recipients</code></td>
    <td>array</td>
    <td><code>["all@example.com"]</code></td>
    <td>Array of the recipients the plugin should send emails.</td>
  </tr>

  <tr>
    <td><code>log.output</code></td>
    <td>string</td>
    <td><code>"stdout"</code></td>
    <td>
      Logger output. Could be <code>"stdout"</code>, <code>"stderr"</code> or a file name,
      eg. <code>"/var/lib/teleport/gitlab.log"</code>
    </td>
  </tr>
  <tr>
    <td><code>log.severity</code></td>
    <td>string</td>
    <td><code>"INFO"</code></td>
    <td>
      Logger severity. Possible values are <code>"INFO"</code>, <code>"ERROR"</code>,
      <code>"DEBUG"</code> or <code>"WARN"</code>.
    </td>
  </tr>
</table>
