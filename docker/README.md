## Docker

This directory contains a set of tools to run Teleport, Teleport Plugins, and
Teleport Terraform Provider locally in Docker.

### TOC

- [Prerequisites](#prerequisites)
- [Setup](#setup)
- [Starting](#starting)
- [Stopping](#stopping)
- [Testing and Manual QA](#testing)
- [Adding new plugins](#adding-a-new-plugin)

### Prerequisites

This guide assumes you'll run the QA on a machine that has the following:

- GNU Make
- Docker
- git

### Setup

This flow builds on top of
[Teleport's own Docker flow](https://github.com/gravitational/teleport/tree/master/docker).
Teleport's own Docker image and services are managed with that flow.

#### Overview

You'll need to build Docker images for:

- Teleport Enterprise
- Teleport Plugins
- Terraform (terraform commands will be executer from that VM).

The flow is structured in the following way:

- `Dockerfile` is responsible for _installing_ the software that we'll run.
- `docker-compose.yml` is responsible for baseline configuration for the cluster
  to work together, and for passing runtime params to the containers.
- `make config-*` subcommands will help you setup specific configs for specific
  plugins.

#### Getting started with Teleport's Docker flow

First prepare your teleport directory to work with the Docker flow. The flow
assumes that you have `teleport` alongside `teleport-plugins`, and they have the
same parent directory.

#### Building `teleport:latest`

Go to the teleport source directory and run `make -C docker build`. This should
setup `teleport:latest` image with whatever current runtime is set in Teleport
docker flow source.

```shell
cd ../teleport
make -C docker build
```

**_Note_**: For example, it might use `go1.15.5` — make sure you use the same
runtime version in the whole guide. You might need to adjust the runtime version
in the makefile and docker compose file yourself.

#### Building `teleport-ent:latest`

Teleport Plugins require Enterprise version of Teleport to run correctly, and
`teleport:latest` won't have it by default, so you'll need to build a special
Docker image running the enterprise version. This flow calls this immage
`teleport-ent:latest`, and it's build like this:

```shell
# In your main teleport-plugins/docker directory
make teleport-ent
```

_*Note*: you can pass a `-e RELEASE=binary-teleport-ent-name-to-download` to
docker build command if you want to — that would install a specified Teleport
Enterprise version to the container. All the build does, actually, is it takes
the OSS built `teleport:latest`, downloads the Teleport Enterprise edition, and
installs it._

#### Teleport Enterprise License

_*Note*: this setup requires you to bring your own Teleport Enterprise License
and put it to `data/var/lib/teleport/license.pam`. Enterprise features,
specifically creating roles with tctl, and hence the whole flow, might not work
otherwise._

#### Building plugins and their docker images

The flow uses the code in your `teleport-plugins` repo clone to build and run
the plugins. To test different versions, redo this part for a different branch.

To prepare the plugins, we'll build them all in the build box (the Docker image
used to build Teleport itself), and then, we'll use a separate Docker image to
run those plugins that we've just built, using the same architecture as we used
for the buildbox.

`make plugins` will first run the buildbox to build all the plugins, and then
build their docker image.

```bash
make plugins
```

### Starting

#### First start

Before starting testing, you'll need to provision the Teleport cluster
configuration: multiple user roles and accounts for the plugins to work, and
their auth certificates, then export them to /mnt/shared/certs.

The auth certificates will have a rather small TTL by default. If you start the
plugins later and get an auth error, you should run `make config` agian.

```bash
make config
```

#### Starting Teleport and the plugins

```bash
make up # Starts a single node Teleport cluster and all of the available plugins via docker-compose.yml
# OR
docker-compose up teleport teleport-slack # Will only start Teleport and Teleport Slack plugin
```

You can pass CLI flags to both Teleport and every plugin. For example, here's
how to enable debug logging:

```bash
TELEPORT_FLAGS="--debug" PLUGIN_FLAGS="--debug" make up
```

If you prefer to QA without TLS for web proxies and plugin endpoints, you can
disable TLS and pass the `--insecure-no-tls` flags:

```bash
TELEPORT_FLAGS="--insecure-no-tls" PLUGIN_FLAGS="--insecure-no-tls" make up
```

#### Exposing the plugin's HTTP endpoints for webhooks and such

Most of the plugins rely on 3rd party services sending webhooks when some action
happened on their side. For example, teleport-slack expects to receive a webhook
when a request is approved or denied on Slack.

For those webhooks to work correctly, you'd need to expose a public address for
each plugin so that the 3rd party service can reach the plugin running locally
in your test flow.

If you already have a publicly available hostnames for each plugin — great. If
not, you can use the supplied ngrok configurations to expose all of the plugins
like this:

```bash
ngrok start --all --config ./ngrok.yaml
```

Gotchas:

- The repo supplies `ngrok.yaml.example` and `ngrok-insecure.yaml.example`. To
  get started with ngrok, copy those config files and add your own ngrok auth
  key into them.
- You'll need at least the Basic plan with ngrok to run more than one tunnel at
  a time (i.e. test more than a one plugin simultaneously). If you prefer to run
  just one plugin at a time, you can run ngrok like this:
  `ngrok start teleport-slack --config ngrok-insecure.yaml`.
- `ngrok.yaml.example` expects you to use TLS/HTTPS and provide certificates.
  Use `ngrok-insecure.yaml.example` with `--insecure-no-tls` option to test the
  plugins without HTTPS.

### Stopping

```bash
make down
```

### Testing Plugins

Here's a good starting point on what needs to be tested before each release of
the plugins: [Test Plan](/testplan.md).

After setting everything up and starting Teleport and the plugins for the first
time with the default config files, you'll notice that most of them fail their
API healthchecks and exit. **You'll need to provide valid plugin configs,
specifically any 3rd party OAuth application credentials and URLs for the
plugins to work and to be tested**.

Two major things to provide:

- Any application credentials, like Slack app ID and app secret, Gitlab URL and
  app id & secret, etc.
- The flow does not provision public URLs for the plugins. You'll need to setup
  public URLs with `ngrok` or another server / service, and set those URLs up in
  `public_addr` setting of each corresponding plugin. **Note that the default
  config does map ports for each plugin to your local testing machine, so you
  might just want to expose that machine with a public address and use it's
  address and corresponding ports**.

The flow is designed to provide two points where you can change the
configuration of the cluster to test different scenarios:

- `docker/docker-compose.yml` to change the cluster layout (which services to
  run). For example, if you'd want to run all of the services with
  `--insecure-no-tls` flag, adding the flag to the corresponding services
  `command:` key would work.
- `docker/teleport/*.yaml` configures the Teleport service itself.
- `docker/plugins/teleport-*.yaml` configures the plugins. **You'll most likely
  want to edit any additional plugin configuration, like OAuth app
  credentials.**

## Contributing

### Adding a new plugin

1. Make sure that the plugin will be built by invoking
   `make -C docker build-plugins`. The easiest way to do that, is to make sure
   that is to ensure that the [Makefile in teleport-plugins](/Makefile) repo
   root will build it when `build-all` is invoked. The new plugin will use the
   same Docker image as the other plugins.
2. Add a new service to the `docker/docker-compose.yml` for the plugin to be
   started on `make up` or `docker-compose up`.
3. Create a configuration file for the plugin. Run the plugin's `configure`
   command and save that into a new configuration file, then edit that.
