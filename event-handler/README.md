# Teleport-event-handler

This plugin is used to export Audit Log events to Fluentd service.

## Setup

### Prerequisites

This guide assumes that you have:
* Teleport 6.2 or newer
* Admin privileges to access tctl
* Docker to build plugin from source and run fluentd example instance

The required Fluentd version for production setup is v1.12.4 or newer. Lower versions do not support TLS.

## Install the plugin

There are several methods to installing and using the Teleport Event Handler Plugin:

1. Use a [precompiled binary](#precompiled-binary)

2. Use a [docker image](#docker-image)

3. Install from [source](#building-from-source)

### Precompiled Binary

Get the plugin distribution.

```bash
$ curl -L https://get.gravitational.com/teleport-event-handler-v7.0.2-linux-amd64-bin.tar.gz
$ tar -xzf teleport-event-handler-v7.0.2-linux-amd64-bin.tar.gz
$ cd teleport-event-handler
$ ./install
```

### Docker Image
```bash
$ docker pull quay.io/gravitational/teleport-plugin-event-handler:9.0.2
```

```bash
$ docker run quay.io/gravitational/teleport-plugin-event-handler:9.0.2 version
Teleport event handler v9.0.2 git:teleport-event-handler-v9.0.2-0-g9e149895 go1.17.8
```

For a list of available tags, visit [https://quay.io/](https://quay.io/gravitational/teleport-plugin-event-handler?tab=tags)

### Building from source

Please ensure that Docker is running!

```sh
$ git clone https://github.com/gravitational/teleport-plugins.git --depth 1
$ cd teleport-plugins/event-handler/build.assets
$ make install
```

This command will build `build/teleport-event-handler` executable and place it to `/usr/local/bin` folder. The following error means that you do not have write permissions on target folder:

```sh
cp: /usr/local/bin/teleport-event-handler: Operation not permitted
```

To fix this, you can either set target folder to something listed in your `$PATH`:

```sh
$ make install BINDIR=/tmp/test-fluentd-setup
```

or copy binary file manually with `sudo`:

```sh
$ sudo cp build/teleport-event-handler /usr/local/bin
```

## Generate example configuration

Run:

```sh
$ teleport-event-handler configure .
```

You'll see the following output:

```sh
Teleport event handler 9.0.2 teleport-event-handler-v9.0.2-0-g9e149895

[1] Generated mTLS Fluentd certificates ca.crt, ca.key, server.crt, server.key, client.crt, client.key
[2] Generated sample teleport-event-handler role and user file teleport-event-handler-role.yaml
[3] Generated sample fluentd configuration file fluent.conf
[4] Generated plugin configuration file teleport-event-handler.toml

Follow-along with our getting started guide:

https://goteleport.com/docs/setup/guides/fluentd
```

Where `ca.crt` and `ca.key` would be Fluentd self-signed CA certificate and private key, `server.crt` and `server.key` would be fluentd server certificate and key, `client.crt` and `client.key` would be Fluentd client certificate and key, all signed by the generated CA.

Check ```teleport-event-handler configure --help``` usage instructions. You may set several configuration options, including key/cert file names, server key encryption password and Teleport auth proxy address.

## Create user and role for access audit log events

The generated `teleport-event-handler-role.yaml` would contain the following content:

```yaml
kind: user
metadata:
  name: teleport-event-handler
spec:
  roles: ['teleport-event-handler']
version: v2
---
kind: role
metadata:
  name: teleport-event-handler
spec:
  allow:
    rules:
      - resources: ['event','session']
        verbs: ['list','read']
version: v5
```

It defines `teleport-event-handler` role and user which has read-only access to the `event` API.

Log into Teleport Authentication Server, this is where you normally run `tctl`. Run `tctl` to create role and user:

```sh
tctl create -f teleport-event-handler-role.yaml
```

## <a name="export"></a>Export teleport-event-handler identity file

Teleport Plugin use the fluentd role and user to read the events. We export the identity files, using tctl auth sign.

```sh
tctl auth sign --out identity --user teleport-event-handler
```

This will generate `identity` which contains TLS certificates and will be used to connect plugin to your Teleport instance.

## Run fluentd

The plugin will send events to the fluentd instance using keys generated on the previous step. Generated `fluent.conf` file would contain the following content:

```
<source>
    @type http
    port 8888

    <transport tls>
        client_cert_auth true

        # We are going to run fluentd in Docker. /keys will be mounted from the host file system.
        ca_path /keys/ca.crt
        cert_path /keys/server.crt
        private_key_path /keys/server.key
        private_key_passphrase ********** # Passphrase generated along with the keys
    </transport>

    <parse>
      @type json
      json_parser oj

      # This time format is used by the plugin. This field is required.
      time_type string
      time_format %Y-%m-%dT%H:%M:%S
    </parse>
</source>

# Events sent to test.log will be dumped to STDOUT.
<match test.log> 
  @type stdout
</match>

# Events sent to session.*.log will be dumped to STDOUT.
<match session.*.log> 
  @type stdout
</match>
```

Start fluentd instance:

```sh
docker run -p 8888:8888 -v $(pwd):/keys -v $(pwd)/fluent.conf:/fluentd/etc/fluent.conf fluent/fluentd:edge 
```

## Configure the plugin

The generated `teleport-event-handler.toml` would contain the following plugin configuration:

```toml
storage = "./storage" # Plugin will save it's state here
timeout = "10s"
batch = 20
namespace = "default"

[fluentd]
cert = "client.crt"
key = "client.key" 
ca = "ca.crt"
url = "https://localhost:8888/test.log"
session-url = "https://localhost:8888/session" # .<session id>.log will be appended to this URL

[teleport]
addr = "localhost:3025" # Default local Teleport instance address
identity = "identity"   # Identity file exported on previous step
```

## Start the plugin

```sh
$ teleport-event-handler start --config teleport-event-handler.toml --start-time 2021-01-01T00:00:00Z
```

or with docker:

```sh
$ docker run -v </path/to/config>:/etc/teleport-event-handler quay.io/gravitational/teleport-plugin-event-handler:9.0.2 start --config /etc/teleport-event-handler/teleport-event-handler.toml --start-time 2021-01-01T00:00:00Z
```

Note that here we used start time at the beginning of year 2021. Supposedly you have some events at the Teleport instance you are connecting to. Otherwise, you can omit `--start-time` flag, start the service and generate an events using `tctl create -f teleport-event-handler.yaml` then from the first step. `teleport-event-handler` will wait for that new events to appear and will send them to the fluentd.

You should see something like this:

```sh
INFO[0046] Event sent                                    id=0b5f2a3e-faa5-4d77-ab6e-362bca0994fc ts="2021-06-08 11:00:56.034 +0000 UTC" type=user.login
INFO[0046] Event sent                                    id=8a435f89-a70a-4bb4-9b0f-2818da51a62b ts="2021-06-08 12:09:11.344 +0000 UTC" type=user.create
INFO[0046] Event sent                                    id=04734bc5-f8d8-493f-8109-680b8df76ce9 ts="2021-06-08 12:09:11.783 +0000 UTC" type=role.created
INFO[0046] Event sent                                    id=2a3ac443-5e32-41c7-9b3e-da45d53f27b2 ts="2021-06-08 12:09:43.892 +0000 UTC" type=user.update
INFO[0046] Event sent                                    id=af9c0777-7f02-4ec4-a682-3896a6960ce5 ts="2021-06-08 12:09:44.329 +0000 UTC" type=role.created
```

## Do not forget to set time range

By default, all events starting from the current moment will be exported. If you want to export previous events, you have to pass `--start-time` CLI arg.

This will start an export from May 5 2021:

```sh
teleport-event-handler start --config teleport-event-handler.toml --start-time "2021-05-05T00:00:00Z"
```

This will export new events from a moment the service did start:

```sh
teleport-event-handler start --config teleport-event-handler.toml
```

Note that start time can be set only once, on the first run of the tool. If you want to change the time frame later, remove plugin state dir which you had specified in `storage-dir` argument.s

## How it works

* `teleport-event-handler` takes the Audit Log event stream from Teleport. It loads events in batches of 20 by default. Every event gets sent to fluentd.
* Once event is successfully received by fluentd, it's ID is saved to the `teleport-event-handler` state. In case `teleport-event-handler` crashes, it will pick the stream up from a latest successful event.
* Once all events are sent, `teleport-event-handler` starts polling for new evetns. It happens every 5 seconds by default.
* If storage directory gets lost, you may specify latest event id value. `teleport-event-handler` will pick streaming up from the next event after it.

## Configuration options

You may specify configuration options via command line arguments, environment variables or TOML file.

| CLI arg name        | Description                                         | Env var name              |
| --------------------|-----------------------------------------------------|---------------------------|
| teleport-addr       | Teleport host and port                              | FDFWD_TELEPORT_ADDR       |
| teleport-ca         | Teleport TLS CA file                                | FDFWD_TELEPORT_CA         |
| teleport-cert       | Teleport TLS certificate file                       | FDWRD_TELEPORT_CERT       |
| teleport-key        | Teleport TLS key file                               | FDFWD_TELEPORT_KEY        |
| teleport-identity   | Teleport identity file                              | FDFWD_TELEPORT_IDENTITY   |
| fluentd-url         | Fluentd URL                                         | FDFWD_FLUENTD_URL         |
| fluentd-session-url | Fluentd session URL                                 | FDFWD_FLUENTD_SESSION_URL |
| fluentd-ca          | fluentd TLS CA file                                 | FDFWD_FLUENTD_CA          |
| fluentd-cert        | Fluentd TLS certificate file                        | FDFWD_FLUENTD_CERT        |
| fluentd-key         | Fluentd TLS key file                                | FDFWD_FLUENTD_KEY         |
| storage             | Storage directory                                   | FDFWD_STORAGE             |
| batch               | Fetch batch size                                    | FDFWD_BATCH               |
| namespace           | Events namespace                                    | FDFWD_NAMESPACE           |
| types               | Comma-separated list of event types to forward      | FDFWD_TYPES               |
| skip-session-types  | Comma-separated list of session event types to skip | FDFWD_SKIP_SESSION_TYPES  |
| start-time          | Minimum event time (RFC3339 format)                 | FDFWD_START_TIME          |
| timeout             | Polling timeout                                     | FDFWD_TIMEOUT             |
| cursor              | Start cursor value                                  | FDFWD_CURSOR              |
| debug               | Debug logging                                       | FDFWD_DEBUG               |

TOML configuration keys are the same as CLI args. Teleport and Fluentd variables can be grouped into sections. See [example TOML](example/config.toml). You can specify TOML file location using `--config` CLI flag.

You could use `--dry-run` argument if you want event handler to simulate event export (it will not connect to Fluentd). `--exit-on-last-event` can be used to terminate service after the last event is processed.

`--skip-session-types` is `['print']` by default. Please note that if you enable forwarding of print events (`--skip-session-types=''`) the `Data` field would also be sent.

## Using with Teleport Cloud

### Login to Teleport cloud:

```sh
$ tsh login --proxy test.teleport.sh:443 --user test@evilmartians.com
```

### Generate sample configuration using the cloud address:

```sh
$ teleport-event-handler configure . test.teleport.sh:443
```

Then follow the manual starting at ["Export teleport-event-handler identity file"](#export) section.

## Advanced topics

### <a name="mtls_advanced"></a>Generate mTLS certificates using OpenSSL/LibreSSL

For the purpose of security, we require mTLS to be enabled on the fluentd side. You are going to need [OpenSSL configuration file](example/ssl.conf). Put the following contents to `ssl.conf`:

```sh
[req]
default_bits        = 4096
distinguished_name  = req_distinguished_name
string_mask         = utf8only
default_md          = sha256
x509_extensions     = v3_ca

[req_distinguished_name]
countryName                     = Country Name (2 letter code)
stateOrProvinceName             = State or Province Name
localityName                    = Locality Name
0.organizationName              = Organization Name
organizationalUnitName          = Organizational Unit Name
commonName                      = Common Name
emailAddress                    = Email Address

countryName_default             = US
stateOrProvinceName_default     = USA
localityName_default            =
0.organizationName_default      = Teleport
commonName_default              = localhost

[v3_ca]
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always,issuer
basicConstraints = critical, CA:true, pathlen: 0
keyUsage = critical, cRLSign, keyCertSign

[client_cert]
basicConstraints = CA:FALSE
nsCertType = client, email
nsComment = "OpenSSL Generated Client Certificate"
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer
keyUsage = critical, nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth, emailProtection

[server_cert]
basicConstraints = CA:FALSE
nsCertType = server
nsComment = "OpenSSL Generated Server Certificate"
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer:always
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = DNS:localhost,IP:127.0.0.1

[crl_ext]
authorityKeyIdentifier=keyid:always

[ocsp]
basicConstraints = CA:FALSE
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer
keyUsage = critical, digitalSignature
extendedKeyUsage = critical, OCSPSigning
```

Generate certificates using the following commands:

```sh
openssl genrsa -out ca.key 4096
chmod 444 ca.key
openssl req -config ssl.conf -key ca.key -new -x509 -days 7300 -sha256 -extensions v3_ca -subj "/CN=ca" -out ca.crt

openssl genrsa -aes256 -out server.key 4096
chmod 444 server.key
openssl req -config ssl.conf -subj "/CN=server" -key server.key -new -out server.csr
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 365 -out server.crt -extfile ssl.conf -extensions server_cert

openssl genrsa -out client.key 4096
chmod 444 client.key
openssl req -config ssl.conf -subj "/CN=client" -key client.key -new -out client.csr
openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 365 -out client.crt -extfile ssl.conf -extensions client_cert
```

You will be requested to enter key password. Remember this password since it will be required later, in fluentd configuration. Note that for the testing purposes we encrypt only `server.key` (which is fluentd instance key). It is strongly recommended by the Fluentd. Plugin does not yet support client key encryption.

Alternatively, you can run: `PASS=12345678 KEYLEN=4096 make gen-example-mtls` from the plugin source folder. Keys will be generated and put to `example/keys` folder.