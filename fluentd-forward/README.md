# Fluentd-forward

This plugin is used to export Audit Log events to Fluentd service.

## Setup

### Prerequisites

This guide assumes that you have:
* Teleport 6.2 or newer
* Admin privileges to access tctl
* OpenSSL or LibreSSL (macOS)
* Docker to build plugin from source and run fluentd example instance

The required Fluentd version for production setup is v1.12.4 or newer. Lower versions do not support TLS.

### Create user and role for access audit log events

Log into Teleport Authentication Server, this is where you normally run tctl. Create a new user and role that only has read-only API access to the `event` API. The below script will create a [yaml resource file](example/fluentd-forward.yaml) for a new user and role.

```yaml
kind: user
metadata:
  name: fluentd-forward
spec:
  roles: ['fluentd-forward']
version: v2
---
kind: role
metadata:
  name: fluentd-forward
spec:
  allow:
    rules:
      - resources: ['event']
        verbs: ['list','read']
version: v3
```

Here and below follow along and create yaml resources using `tctl create -f`:

```sh
tctl create -f fluentd-forward.yaml
```

### Export fluentd-forward Certificate

Teleport Plugin use the fluentd role and user to read the events. We export the identity files, using tctl auth sign.

```sh
tctl auth sign --format=tls --user=fluentd-forward --out=fd --ttl=720h
```

This will generate `fd.cas`, `fd.crt` and `fd.key` which will be used to connect plugin to your Teleport instance.

## Installing the plugin

We recommend installing the Teleport Plugins alongside the Teleport Proxy. This is an ideal location as plugins have a low memory footprint, and will require both public internet access and Teleport Auth access. We currently only provide linux-amd64 binaries, you can also compile these plugins from source.

### Install the plugin from source

```sh
git clone https://github.com/gravitational/teleport-plugins.git --depth 1
cd teleport-plugins/fluentd-forward/build.assets
make install
```

This will place `fluentd-forward` executable to `/usr/local/bin` folder. You can override the target directory:

```sh
make install BINDIR=/tmp/test-fluentd-setup
```

### Configure the plugin

Save the following content to `fluentd-forward.toml`:

```toml
storage = "./fluentd-forward-storage" # Plugin will save it's state here
timeout = "10s"
batch = 10
namespace = "default"

[fluentd]
cert = "client.crt"
key = "client.key" 
ca = "ca.crt"
url = "https://localhost:8888/test.log"

[teleport]
addr = "localhost:3025"
ca = "fd.cas" 
cert = "fd.crt"
key  = "fd.key"
```

## Setup fluentd

For the purpose of testing, we will run the local fluentd instance first.

### Generate mTLS certificates

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

Alternatively, you can run: `PASS=12345678 make gen-example-mtls` from the plugin source folder. Keys will be generated and put to `example/keys` folder.

### Configure fluentd

The plugin will send events to the fluentd instance using keys generated on the previous step. Put the following contents into `fluent.conf`:

```
<source>
    @type http
    port 8888

    <transport tls>
        client_cert_auth true
        ca_path /keys/ca.crt
        cert_path /keys/server.crt
        private_key_path /keys/server.key
        private_key_passphrase passphrase # Specify password used to generate keys
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
```

Please notice that passphrase must be changed to the one you used during key generation.

## Run test setup

* Start `fluentd`:

```sh
docker run -p 8888:8888 -v $(pwd):/keys -v $(pwd)/fluent.conf:/fluentd/etc/fluent.conf fluent/fluentd:edge 
```

* Start `fluentd-forward`:

```sh
fluentd-forward --config fluentd-forward.toml -d
```

`-d` flag is used to enable debug logging.

You should see something like this:

```sh
DEBU[0010] JSON to send                                  json="{\"ei\":0,\"event\":\"role.created\",\"uid\":\"4f3cc272-4d54-4729-8563-20702cac0d4b\",\"code\":\"T9000I\",\"time\":\"2021-05-26T11:15:30.587Z\",\"cluster_name\":\"teleport-cluster\",\"name\":\"terraform\",\"expires\":\"0001-01-01T00:00:00Z\",\"user\":\"79e2cc83-8d4f-4897-84a2-ccd3427246b7.teleport-cluster\"}"
INFO[0010] Event sent                                    fields.time="2021-05-26 11:15:30.587 +0000 UTC" type=role.created
DEBU[0010] Event dump                                    event="Metadata:<Type:\"role.created\" ID:\"4f3cc272-4d54-4729-8563-20702cac0d4b\" Code:\"T9000I\" Time:<seconds:1622027730 nanos:587000000 > ClusterName:\"teleport-cluster\" > Resource:<Name:\"terraform\" Expires:<seconds:-62135596800 > > User:<User:\"79e2cc83-8d4f-4897-84a2-ccd3427246b7.teleport-cluster\" > "
DEBU[0010] JSON to send                                  json="{\"ei\":0,\"event\":\"user.create\",\"uid\":\"e01f9e74-fea3-4fd0-b8e5-638264fbae27\",\"code\":\"T1002I\",\"time\":\"2021-06-01T11:07:12.536Z\",\"cluster_name\":\"teleport-cluster\",\"user\":\"79e2cc83-8d4f-4897-84a2-ccd3427246b7.teleport-cluster\",\"name\":\"fluentd-forward\",\"expires\":\"0001-01-01T00:00:00Z\",\"roles\":[\"fluentd-forward\"],\"connector\":\"local\"}"
```

## How it works

* `fluentd-forward` takes the Audit Log event stream from Teleport. It loads events in batches of 20 by default. Every event gets sent to fluentd.
* Once event is successfully received by fluentd, it's ID is saved to the `fluent-forward` state. In case `fluentd-forward` crashes, it will pick the stream up from a latest successful event.
* Once all events are sent, `fluentd-forward` starts polling for new evetns. It happens every 5 seconds by default.
* If storage directory gets lost, you may specify latest event id value. `fluentd-forward` will pick streaming up from the next event after it.

## Configuration options

You may specify configuration options via command line arguments, environment variables or TOML file.

| CLI arg name       | Description                                    | Env var name         |
| -------------------|------------------------------------------------|----------------------|
| teleport-addr      | Teleport host and port                         | FDFWD_TELEPORT_ADDR  |
| teleport-ca        | Teleport TLS CA file                           | FDFWD_TELEPORT_CA    |
| teleport-cert      | Teleport TLS certificate file                  | FDWRD_TELEPORT_CERT  |
| teleport-key       | Teleport TLS key file                          | FDFWD_TELEPORT_KEY   |
| fluentd-url        | Fluentd url                                    | FDFWD_FLUENTD_URL    |
| fluentd-ca         | fluentd TLS CA file                            | FDFWD_FLUENTD_CA     |
| fluentd-cert       | Fluentd TLS certificate file                   | FDFWD_FLUENTD_CERT   |
| fluentd-key        | Fluentd TLS key file                           | FDFWD_FLUENTD_KEY    |
| storage            | Storage directory                              | FDFWD_STORAGE        |
| batch              | Fetch batch size                               | FDFWD_BATCH          |
| namespace          | Events namespace                               | FDFWD_NAMESPACE      |
| types              | Comma-separated list of event types to forward | FDFWD_TYPES          |
| start-time         | Minimum event time (RFC3339 format)            | FDFWD_START_TIME     |
| timeout            | Polling timeout                                | FDFWD_TIMEOUT        |
| cursor             | Start cursor value                             | FDFWD_CURSOR         |

TOML configuration keys are the same as CLI args. Teleport and Fluentd variables can be grouped into sections. See [example TOML](example/config.toml).