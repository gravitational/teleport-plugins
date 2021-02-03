## Docker

This directory contains a set of tools to run Teleport, Teleport Plugins, and
Teleport Terraform Provider locally in Docker.

### Getting started

The flow needs `teleport-ent` image to run Teleport Enterprise, and
`teleport-slack` and other plugin images to run the plugins.

The provided `docker-compose.yml` uses
[publicly available images on Quay for teleport enterprise](https://quay.io/repository/gravitational/teleport-ent?tag=latest&tab=tags).

This flow also uses [teleport-buildbox](quay.io/gravitational/teleport-buildbox)
to build plugins in that container.

#### Building plugins and their docker images

`make plugins` will first run the buildbox to build all the plugins, and then
build their docker image.

```bash
make plugins
```

#### Teleport Enterprise License

_*Note*: this setup requires you to bring your own Teleport Enterprise License
and put it to `data/var/lib/teleport/license.pam`. Enterprise features,
specifically creating roles with tctl, and hence the whole flow, might not work
otherwise._

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
