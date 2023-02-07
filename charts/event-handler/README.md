# Teleport Event Handler Plugin

This chart sets up and configures a Deployment for the Event Handler plugin.

## Installation

### Prerequisites

First, you'll need to create a Teleport user and role for the plugin. The following file contains a minimal user that's needed for the plugin to work:

```yaml
---
kind: role
version: v6
metadata:
  name: teleport-plugin-event-handler
spec:
  allow:
    logins:
    - teleport-plugin-event-handler
    rules:
    - resources:
      - event
      verbs:
      - list
      - read
  options:
    forward_agent: false
    max_session_ttl: 8760h0m0s
    port_forwarding: false
---
kind: user
version: v2
metadata:
  name: teleport-plugin-event-handler
spec:
  roles:
    - teleport-plugin-event-handler
```

You can either create the user and the roles by putting the YAML above into a file and issuing the following command  (you must be logged in with `tsh`):

```console
tctl create user.yaml
```

or by navigating to the Teleport Web UI under `https://<yourserver>/web/users` and `https://<yourserver>/web/roles` respectively. You'll also need to create a password for the user by either clicking `Options/Reset password...` under `https://<yourserver>/web/users` on the UI or issuing `tctl users reset teleport-plugin-event-handler` in the command line.

The next step is to create an identity file, which contains a private/public key pair and a certificate that'll identify us as the user above. To do this, log in with the newly created credentials and issue a new certificate (525600 and 8760 are both roughly a year in minutes and hours respectively):

```console
tsh login --proxy=proxy.example.com --auth local --user teleport-plugin-event-handler --ttl 525600
```

```console
tctl auth sign --user teleport-plugin-event-handler --ttl 8760h --out teleport-plugin-event-handler-identity
```

Alternatively, you can execute the command above on one of the `auth` instances/pods.

The last step is to create the secret. The following command will create a Kubernetes secret with the name `teleport-plugin-event-handler-identity` with the key `auth_id` in it holding the contents of the file `teleport-plugin-event-handler-identity`:

```console
kubectl create secret generic teleport-plugin-event-handler-identity --from-file=auth_id=teleport-plugin-event-handler-identity
```

### Mounting Fluentd client certificate

See the [plugin's documentation](../../event-handler/README.md#mtls_advanced) about how to generate the certificates using fluentd's CA certificate and private key.

Once the files `client.key` and `client.crt` were created successfully, the following command can be used to create a new secret (`ca.crt` is also included since we'll need it to verify we are connecting to the right fluentd):

```console
kubectl create secret generic teleport-plugin-event-handler-client-tls --from-file="ca.crt=ca.crt,client.key=client.key,client.crt=client.crt"
```

### Storage

The `event-handler` plugin stores it's current state on the disk, so to avoid accidentally sending any events twice, it's a good idea to create a PersistentVolumeClaim to ensure those files are retained. The Chart contains a default implementation (see the example values file below). Check out [Settings](#settings) for more options!

For more information, check out [Persistent Volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) in the Kubernetes Documentation.

### Installing the plugin

```console
helm repo add teleport https://charts.releases.teleport.dev/
```

```console
helm install teleport-plugin-event-handler teleport/teleport-plugin-event-handler --values teleport-plugin-event-handler-values.yaml
```

Example `teleport-plugin-event-handler-values.yaml`:

```yaml
teleport:
  address: teleport.example.com:443
  identitySecretName: teleport-plugin-event-handler-identity

eventHandler:
  storagePath: "/var/lib/teleport/plugins/event-handler/storage"
  timeout: "10s"
  batch: 20
  namespace: "default"

fluentd:
  url: "https://fluentd.fluentd.svc.cluster.local/events.log"
  sessionUrl: "https://fluentd.fluentd.svc.cluster.local/session.log"
  certificate:
    secretName: "teleport-plugin-event-handler-client-tls"
    caPath: "ca.crt"
    certPath: "client.crt"
    keyPath: "client.key"

persistentVolumeClaim:
  enabled: true
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
    <td><code>eventHandler.storagePath</code></td>
    <td>Path to the directory where <code>event-handler</code>'s state is stored</td>
    <td>string</td>
    <td><code>"/var/lib/teleport/plugins/event-handler/storage"</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>eventHandler.timeout</code></td>
    <td>Maximum time to wait for incoming events before sending them to fluentd.</td>
    <td>string</td>
    <td><code>"10s"</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>eventHandler.batch</code></td>
    <td>Maximum number of events fetched from Teleport in one request</td>
    <td>string</td>
    <td><code>20</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>eventHandler.namespace</code></td>
    <td>Namespace where the events are received from</td>
    <td>string</td>
    <td><code>20</code></td>
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
    <td>Path of the CA certificate in the secret described by <code>fluentd.secretName</code>.</td>
    <td>string</td>
    <td><code>"ca.crt"</code></td>
  </tr>
  <tr>
    <td><code>fluentd.certPath</code></td>
    <td>Path of the client's certificate in the secret described by <code>fluentd.secretName</code>.</td>
    <td>string</td>
    <td><code>"client.crt"</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>fluentd.keyPath</code></td>
    <td>Path of the client private key in the secret described by <code>fluentd.secretName</code>.</td>
    <td>string</td>
    <td><code>"client.key"</code></td>
    <td>no</td>
  </tr>

  <tr>
    <td><code>persistentVolumeClaim.enabled</code></td>
    <td>
      Instructs the Helm chart to include a PersistentVolumeClaim for the storage. This storage
      will be mounted to the path specified by <code>eventHandler.storagePath</code>.
    </td>
    <td>boolean</td>
    <td><code>false</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>persistentVolumeClaim.size</code></td>
    <td>Sets the size of the created PersistentVolumeClaim. Don't forget to append the proper suffix!</td>
    <td>string</td>
    <td><code>"1Gi"</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>persistentVolumeClaim.storageClassName</code></td>
    <td>
      Sets the storage class name of the created PersistentVolumeClaim. Kubernetes will use the default
      one when omitted.
    </td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
  </tr>
  <tr>
    <td><code>persistentVolumeClaim.existingClaim</code></td>
    <td>
      Specifies an already existing PersistentVolumeClaim which should be mounted to the path specified
      by <code>eventHandler.storagePath</code>. <code>persistentVolumeClaim.enabled</code> must be set to false for this
      option to take precedence. Ignored when <code>persistentVolumeClaim.enabled</code> is true.
    </td>
    <td>string</td>
    <td><code>""</code></td>
    <td>no</td>
  </tr>
</table>
