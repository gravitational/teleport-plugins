# Teleport Event Handler Plugin

This chart sets up and configures a Deployment for the Event Handler plugin.

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
    <td><code>fluentd.url</code></td>
    <td>URL of fluentd where the event logs will be sent to.</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>fluentd.sessionUrl</code></td>
    <td>URL of fluentd where the session logs will be sent to.</td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>fluentd.secretName</code></td>
    <td>
      Name of the secret where credentials for the connection is stored.
      It must contain the client's private key, certificate and fluentd's
      CA certificate. See the default paths below.
    </td>
    <td>string</td>
    <td><code>""</code></td>
    <td>yes</td>
  </tr>
  <tr>
    <td><code>fluentd.caPath</code></td>
    <td>Path of the CA certificate in the secret described by `fluentd.secretName`.</td>
    <td>string</td>
    <td><code>"ca.crt"</code></td>
  </tr>
  <tr>
    <td><code>fluentd.certPath</code></td>
    <td>Path of the client's certificate in the secret described by `fluentd.secretName`.</td>
    <td>string</td>
    <td><code>"client.crt"</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>fluentd.keyPath</code></td>
    <td>Path of the client private key in the secret described by `fluentd.secretName`.</td>
    <td>string</td>
    <td><code>"client.key"</code></td>
    <td>no</td>
  </tr>
</table>
