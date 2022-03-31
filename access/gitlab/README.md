# Teleport GitLab Plugin

The plugin allows teams to setup permissions workflow over their existing or new
GitLab projects. When someone requests new roles in Teleport, an issue will be opened,
and the team members can assign approval or denied label to the issue to approve
or deny the request.

## Setup

### Install the plugin

Get the plugin distribution.

```bash
$ curl -L https://get.gravitational.com/teleport-access-gitlab-v7.0.2-linux-amd64-bin.tar.gz
$ tar -xzf teleport-access-gitlab-v7.0.2-linux-amd64-bin.tar.gz
$ cd teleport-access-gitlab
$ ./install
```

### Set up GitLab project & API token

1. On GitLab, go "User Settings" -> "Access Tokens". Create a token with api
   scope, remember the token.
2. Create a project, get its numeric "Project ID" from "Project Overview" ->
   "Details" page.
3. You might want to create the Board with lists: `Teleport: Pending`,
   `Teleport: Approved`, and `Teleport: Denied`. The plugin will work if you
   just change labels on issues, but with a Board you can just drag the issue
   into a status-column you want.

### Teleport User and Role

Using Web UI or `tctl` CLI utility, create the role `access-gitlab` and the user `access-gitlab` belonging to the role `access-gitlab`. You may use the following YAML declarations.

#### Role

```yaml
kind: role
metadata:
  name: access-gitlab
spec:
  allow:
    rules:
      - resources: ['access_request']
        verbs: ['list', 'read', 'update']
version: v5
```

#### User

```yaml
kind: user
metadata:
  name: access-gitlab
spec:
  roles: ['access-gitlab']
version: v2
```

### Generate the certificate

For the plugin to connect to Auth Server, it needs an identity file containing TLS/SSH certificates. This can be obtained with tctl:

```bash
$ tctl auth sign --auth-server=AUTH-SERVER:PORT --format=file --user=access-gitlab --out=/var/lib/teleport/plugins/gitlab/auth_id --ttl=8760h
```

Here, `AUTH-SERVER:PORT` could be `localhost:3025`, `your-in-cluster-auth.example.com:3025`, `your-remote-proxy.example.com:3080` or `your-teleport-cloud.teleport.sh:443`. For non-localhost connections, you might want to pass the `--identity=...` option to authenticate yourself to Auth Server.

### Save configuration file

By default, configuration file is expected to be at `/etc/teleport-gitlab.toml`.

```toml
# /etc/teleport-gitlab.toml
[teleport]
# Teleport Auth/Proxy Server address.
#
# Should be port 3025 for Auth Server and 3080 or 443 for Proxy.
# For Teleport Cloud, should be in the form of "your-account.teleport.sh:443".
addr = "example.com:3025"

# Identity file exported by `tctl auth sign`.
#
identity = "/var/lib/teleport/plugins/gitlab/auth_id"

[db]
path = "/var/lib/teleport/plugins/gitlab/database" # Path to the database file

[gitlab]
url = ""                                   # Leave empty if you are using cloud
token = "token"                            # GitLab API Token
project_id = "1812345"                     # GitLab Project ID
webhook_secret = "your webhook passphrase" # A secret used to encrypt data we use in webhooks. Basically anything you'd like.

[http]
public_addr = "example.com" # URL on which webhook server is accessible externally, e.g. [https://]teleport-gitlab.example.com
# listen_addr = ":8081" # Network address in format [addr]:port on which webhook server listens, e.g. 0.0.0.0:443
https_key_file = "/var/lib/teleport/plugins/gitlab/server.key"  # TLS private key
https_cert_file = "/var/lib/teleport/plugins/gitlab/server.crt" # TLS certificate

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/gitlab.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
```

### Run the plugin

```bash
teleport-gitlab start
```

If something bad happens, try to run it with `-d` option i.e. `teleport-gitlab start -d` and attach the stdout output to the issue you are going to create.

If for some reason you want to disable TLS termination in the plugin and deploy it somewhere else e.g. on some reverse proxy, you may want to run the plugin with `--insecure-no-tls` option. With `--insecure-no-tls` option, plugin's webhook server will talk plain HTTP protocol.

## Building from source

To build the plugin from source you need [Go](https://go.dev/) and `make`.

```bash
git clone https://github.com/gravitational/teleport-plugins.git
cd teleport-plugins/access/gitlab
make
./build/teleport-gitlab start
```
