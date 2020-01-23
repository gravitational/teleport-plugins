## Teleport Plugins Setup Quickstart

If you're using Slack, you can be notified of [new teleport permission requests](https://gravitational.com/teleport/docs/cli-docs/#tctl-request-ls), approve or deny them on Slack with Teleport Slack Plugin. This guide covers it's setup.

For this quickstart, we assume you've already setup an [Enterprise Teleport Cluster](https://gravitational.com/teleport/docs/enterprise/quickstart-enterprise/)

Note: The Approval Workflow only works with Pro and Enterprise version of Teleport.

## Prerequeistios
- An Enterprise or Pro Teleport Cluster
- Admin Privileges. With acess and control of [`tctl`](https://gravitational.com/teleport/docs/cli-docs/#tctl)
- Slack Admin Privileges to create an app and install it to your workspace.

### Create an access-plugin role and user within Teleport 
First off, using an exsiting Teleport Cluster, we are going to create a new Teleport User and Role to access Teleport.

#### Create User and Role for access. 
Log into Teleport Authenticaiont Server, this is where you normally run `tctl`. Don't change the username and the role name, it should be `access-plugin` for the plugin to work correctly.

```
$ cat > rscs.yaml <<EOF
kind: user
metadata:
  name: access-plugin
spec:
  roles: ['access-plugin']
version: v2
---
kind: role
metadata:
  name: access-plugin
spec:
  allow:
    rules:
      - resources: ['access_request']
        verbs: ['list','read','update']
    # teleport currently refuses to issue certs for a user with 0 logins,
    # this restriction may be lifted in future versions.
    logins: ['access-plugin']
version: v3
EOF

# ...
$ tctl create -f rscs.yaml
```

#### Export access-plugin Certificate
Teleport Plugin uses the `access-plugin`role and user to peform the approval. We export the identify files, using [`tctl auth sign`](https://gravitational.com/teleport/docs/cli-docs/#tctl-auth-sign).

```
$ tctl auth sign --format=tls --user=access-plugin --out=plug --ttl=8760h
# ...
```

The above sequence should result in three PEM encoded files being generated: plug.crt, plug.key, and plug.cas (certificate, private key, and CA certs respectively).  We'll reference these later when [configuring Teleport-Plugins](#configuration-file).

_Note: by default, tctl auth sign produces certificates with a relatively short lifetime. For production deployments, the --ttl flag can be used to ensure a more practical certificate lifetime. --ttl=8760h exports a 1 year token_
  
### Create Slack App

We'll create a new Slack app and setup it's auth tokens and callback URLs, so that Slack knows how to notify the Teleport plugin when Approve / Deny buttons are clicked.

You'll need to: 
1. Create a new app, pick a name and select a workspace it belongs to. 
2. Select “app features”: we'll enable interactivity and setup the callback URL here. 
3. Add OAuth Scopes. This is required by Slack for the app to be installed — we'll only need a single scope to post messages to your Slack account. 
4. Obtain OAuth token and callback signing secret for the Teleport plugin config. 

#### Creating the app

https://api.slack.com/apps 

App Name: Teleport
Development Slack Workspace: 

![Create Slack App](https://p197.p4.n0.cdn.getcloudapp.com/items/llu4EL7e/Image+2020-01-09+at+10.40.39+AM.png?v=d9750e4fdc77901e0c2ffb2dc6040aee)

#### Setup Interactice Components
In order to receive interaction callbacks, make sure the listen address is publicly accessible and register it with your App under Features > Interactive Components > Request URL.

_Note: you'll setup this URL later in the plugin config file later._

![Interactive Componets](https://p197.p4.n0.cdn.getcloudapp.com/items/v1umg7J0/Image+2020-01-09+at+10.48.08+AM.png?v=5e231f17304f506db13c3ccf445b6682)

### Selecting OAuth Scopes
On the App screen, go to “OAuth and Permissions” under Features in the sidebar menu. Then scroll to Scopes, and add `chat.write` scope so that our plugin can post messages to your Slack channels.

### Add to Workspace

![OAuth Token](https://p197.p4.n0.cdn.getcloudapp.com/items/E0uEg1ol/Image+2020-01-09+at+11.00.23+AM.png?v=1e28ff5bc4f7e0754acc9c7823f354a3)

### Obtain OAuth Token 

![OAuth Token](https://p197.p4.n0.cdn.getcloudapp.com/items/8LuwNQOd/Image+2020-01-21+at+12.49.53+PM.png?v=91552b001daddf469e9f595c4013fa4a)

### Getting the secret signing token

In the sidebar of the app screen, click on Basic. Scroll to App Credentials section, and grab the app's Signing Secret. We'll use it in the config file later.

![Secret Signing Token](https://p197.p4.n0.cdn.getcloudapp.com/items/BluNv0n6/Image+2020-01-21+at+12.52.05+PM.png?v=cdb3688827d1e3cbcb3b5f37dccfdf09)

## Installing 
To start using Teleport Plugins, you will need to Download the binaries and the license file from the customer portal. After downloading the binary tarball, run:

```
$ wget https://get.gravitational.com/teleport-slackbot-v0.0.1-linux-amd64-bin.tar.gz
$ tar -xzf teleport-slackbot-v0.0.1-linux-amd64-bin.tar.gz
$ cd teleport-slackbot
$ ./install
$ which teleport-slackbot
/usr/local/bin/teleport-slackbot
```

### Configuration File
Save the following configuration file as `/etc/teleport-slackbot.toml`.

In the Teleport section, use the certificates you've generated with `tctl auth sign` before. 

In Slack section, use the OAuth token, signing token, setup the desired channel name. The listen URL is the URL the plugin will listen for Slack callbacks. 

_Note: in production, you'll want to have this URL publicly accessible on your infrastructure. For a test run, you can use localhost and then use ngrok to obtain a public URL for that, and use the ngrok URL in the Slack app settings._ 

```TOML
# Example Teleport slackbot TOML configuration file
[teleport]
auth-server = "example.com:3025"  # Auth GRPC API address
client-key = "/var/lib/teleport/plugin/plug.key" # Teleport GRPC client secret key
client-crt = "/var/lib/teleport/plugin/plug.crt" # Teleport GRPC client certificate 
root-cas = "/var/lib/teleport/plugin/plug.cas"     # Teleport cluster CA certs

[slack]
token = "api-token"       # Slack Bot OAuth token
secret = "secret-value"   # Slack API Signing Secret
channel = "channel-name"  # Message delivery channel
listen = "example.com:8081"          # Slack interaction callback listener
```

## Test Run 

`teleport-slackbot start --config=/etc/teleport.toml`

### TSH User Login and Request Admin Role. 
```bash
➜ tsh login --request-roles=admin
Seeking request approval... (id: 8f77d2d1-2bbf-4031-a300-58926237a807)
```

### Setup with SystemD
In production, we recommend starting teleport daemon via an init system like systemd . Here's the recommended Teleport service unit file for systemd: 

```
[Unit]
Description=Teleport Plugin
After=network.target

[Service]
Type=simple
Restart=on-failure
ExecStart=/usr/local/bin/teleport-slackbot start --config=/etc/teleport-slackbot.toml --pid-file=/var/run/teleport.pid
ExecReload=/bin/kill -HUP $MAINPID
PIDFile=/var/run/teleport.pid

[Install]
WantedBy=multi-user.target
```

Save this as `teleport-slackbot.service`. 

# FAQ / Debugging
[TODO]