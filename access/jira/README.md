# Teleport Jira Plugin

This package provides Teleport <-> Jira integration that allows teams to approve
or deny Access Requests on a Jira Project Board. It works with both Jira Cloud
and Jira Server 8.

## Install the plugin

There are several methods to installing and using the Teleport Jira Plugin:

1. Use a [precompiled binary](#precompiled-binary)

2. Use a [docker image](#docker-image)

3. Install from [source](#building-from-source)

### Precompiled Binary

Get the plugin distribution.

```bash
$ curl -L https://get.gravitational.com/teleport-access-jira-v7.0.2-linux-amd64-bin.tar.gz
$ tar -xzf teleport-access-jira-v7.0.2-linux-amd64-bin.tar.gz
$ cd teleport-access-jira
$ ./install
```

### Docker Image
```bash
$ docker pull quay.io/gravitational/teleport-plugin-jira:9.0.2
```

```bash
$ docker run quay.io/gravitational/teleport-plugin-jira:9.0.2 version
teleport-jira v9.0.2 git:teleport-jira-v9.0.2-0-g9e149895 go1.17.8
```

For a list of available tags, visit [https://quay.io/](https://quay.io/repository/gravitational/teleport-plugin-jira?tab=tags)

### Building from source

To build the plugin from source you need [Go](https://go.dev/) and `make`.

```bash
$ git clone https://github.com/gravitational/teleport-plugins.git
$ cd teleport-plugins/access/jira
$ make
$ ./build/teleport-jira start
```

## Set up Jira board

- [See detailed setup instructions for Jira Cloud on the website](https://goteleport.com/docs/enterprise/workflow/ssh-approval-jira-cloud/)
- [See detailed setup instructions for Jira Server on the website](https://goteleport.com/docs/enterprise/workflow/ssh-approval-jira-server/)

Setup process is different for the Jira Cloud and Jira Server editions:

- Please refer to [INSTALL-JIRA-CLOUD.md](./INSTALL-JIRA-CLOUD.md) for a
  detailed Jira Cloud getting started guide.
- Jira Server getting started guide:
  [INSTALL-JIRA-SERVER.md](./INSTALL-JIRA-SERVER.md)

## Teleport User and Role

Using Web UI or `tctl` CLI utility, create the role `access-jira` and the user `access-jira` belonging to the role `access-jira`. You may use the following YAML declarations.

### Role

```yaml
kind: role
metadata:
  name: access-jira
spec:
  allow:
    rules:
      - resources: ['access_request']
        verbs: ['list', 'read', 'update']
version: v5
```

### User

```yaml
kind: user
metadata:
  name: access-jira
spec:
  roles: ['access-jira']
version: v2
```

## Generate the certificate

For the plugin to connect to Auth Server, it needs an identity file containing TLS/SSH certificates. This can be obtained with tctl:

```bash
$ tctl auth sign --auth-server=AUTH-SERVER:PORT --format=file --user=access-jira --out=/var/lib/teleport/plugins/jira/auth_id --ttl=8760h
```

Here, `AUTH-SERVER:PORT` could be `localhost:3025`, `your-in-cluster-auth.example.com:3025`, `your-remote-proxy.example.com:3080` or `your-teleport-cloud.teleport.sh:443`. For non-localhost connections, you might want to pass the `--identity=...` option to authenticate yourself to Auth Server.

## Save configuration file

By default, configuration file is expected to be at `/etc/teleport-jira.toml`.

```toml
# /etc/teleport-jira.toml
[teleport]
# Teleport Auth/Proxy Server address.
#
# Should be port 3025 for Auth Server and 3080 or 443 for Proxy.
# For Teleport Cloud, should be in the form of "your-account.teleport.sh:443".
addr = "example.com:3025"

# Identity file exported by `tctl auth sign`.
#
identity = "/var/lib/teleport/plugins/jira/auth_id"

[db]
# Path to the database file
#
path = "/var/lib/teleport/plugins/jira/database"

[jira]
# Jira URL. For Jira Cloud, URL is of the form "https://[your-jira].atlassian.net":
url = "https://example.com/jira"
# Jira User name:
username = "jira-bot"
# Jira API Basic Auth token, or our password in case you're using Jira Server:
api_token = "token"
# Jira Project key:
project = "MYPROJ"
# Jira Issue type:
issue_type = "Task"

[http]
public_addr = "example.com" # URL on which webhook server is accessible externally, e.g. [https://]teleport-jira.example.com
# listen_addr = ":8081" # Network address in format [addr]:port on which webhook server listens, e.g. 0.0.0.0:443
https_key_file = "/var/lib/teleport/plugins/jira/server.key" # TLS private key
https_cert_file = "/var/lib/teleport/plugins/jira/server.crt" # TLS certificate

[log]
output = "stderr" # Logger output. Could be "stdout", "stderr" or "/var/lib/teleport/jira.log"
severity = "INFO" # Logger severity. Could be "INFO", "ERROR", "DEBUG" or "WARN".
```

## Run the plugin

```bash
$ teleport-jira start
```

or with docker:

```bash
$ docker run -v <path/to/config>:/etc/teleport-jira.toml quay.io/gravitational/teleport-plugin-jira:9.0.2 start
```

If something bad happens, try to run it with `-d` option i.e. `teleport-jira start -d` and attach the stdout output to the issue you are going to create.

If for some reason you want to disable TLS termination in the plugin and deploy it somewhere else e.g. on some reverse proxy, you may want to run the plugin with `--insecure-no-tls` option. With `--insecure-no-tls` option, plugin's webhook server will talk plain HTTP protocol.
