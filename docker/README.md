## Docker

This directory contains Docker-based flow to run Teleport Plugins locally, and
is indended for manual QA with the [Test Plan](../testplan.md) / testing
purposuses.

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
- Public hostname or static IP address, or ngrok.

### Setup

This flow builds on top of
[Teleport's own Docker flow](https://github.com/gravitational/teleport/tree/master/docker).
Teleport's own Docker image and services are managed with that flow.

#### Overview

The setup process builds up several Docker images, and then helps you configure
the Teleport cluster on those images and some plugins to run together.

Docker images are responsible for the software (i.e. have the correct version of
Teleport Enterprise to run the plugins), but the further confuguration, like
what specific certificates and addresses to use for different services is
performed _after Dockerfiles are built_:

- `Dockerfile` is generally responsible for the software running in the
  container.
- `docker-compose.yml` is responsible for baseline configuration for the cluster
  to work together, and for passing runtime params to the containers.
- `make config-*` subcommands will help you setup specific configs for specific
  plugins, and they should work even if the config format changes in the fugure.

#### Getting started with Teleport's Docker flow

First prepare your teleport directory to work with the Docker flow. The flow
assumes that you have `teleport` alongside `teleport-plugins`, and they have the
same parent directory.

Teleport's Docker image uses the `teleport-buildbox` image, so we'll set it up
first. Teleport also uses quay.io to store images, but for the purpose of this
guide, we'll store images locally.

_*Note: if you're also running Teleport's own dockerized testing kit, you may
already have `teleport:lastest` and `teleport-buildox` ready.* If that's the
case, just skip that part and jump to building `teleport-ent` and using it with
the plugins._

#### Building `teleport-buildbox`

The Buildbox is defined and built in `teleport/build.assets` directory. The
easiest way to build it is to run the following:

```bash
git clone git@gitnub.com:gravitational/teleport.git
cd teleport
make -C build.assets build
```

This will build the buildbox container for you, and tag it for quay. If you
don't have access to that, you can run the command that make runs yourself, but
with `-t teleport-builbox:go-{GOVERSION}`, like that:

```bash
# In the teleport/build.assets dir:
docker build \
	--build-arg UID=(id -u) \
	--build-arg GID=(id -g) \
	--build-arg RUNTIME=go1.13.2 \ # Set whatever runtime verison you want
	--cache-from quay.io/gravitational/teleport-buildbox:go1.13.2 \
	-t teleport-buildbox:go1.13.2
	.
```

#### Building `teleport:latest`

You'll need Teleport's `teleport:latest` Docker image to run plugins. This
command will build it:

```bash
# In the parent directory of teleport-plugins
git clone git@gitnub.com:gravitational/teleport.git
cd teleport
make -C docker build
```

Note that unlike the `build.assets` Makefile, `docker/Makefile` tags the image
as `teleport:latest` by default, you don't have to tweak it and remove quay
reference.

After building the Teleport prerequisites, you should have `teleport` and
`teleport-buildbox` images build and tagged like this:

```bash
nate-mbp17:~/g/s/g/g/teleport master docker image ls |grep teleport
teleport                                  latest              9d87869a9aee        50 seconds ago      1.12GB
teleport-buildbox                         go1.14.4            9aea93e15a50        6 minutes ago       866MB
nate-mbp17:~/g/s/g/g/teleport master
```

If you don't have any of those two images built, the flow might not work
correctly.

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

_*Note*: this setup requires you to bring your own Teleport Enterprise License
and put it to `data/var/lib/teleport/license.pam`. Enterprise features, and
hence the whole flow, might not work otherwise._

After that, we'll build the teleport's image from teleport-plugin's directory.
We'll still use teleport's own Dockerfile, but the services in
`docker-compose.yml` are different, and teleport-plugins test flow uses it's own
`data` directory, just so if you've been testing both teleport and
teleport-plugins, they shouldn't interfere with each other.

Please refer to Teleport's Docker documentation for more details about it's
configuration.

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

### Testing

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
