# teleport-plugins

Set of plugins for Teleport


## Access API

The [access](./access) package exposes a simple API for managing access requests
which can be used for writing plugins.  More info can be found in
[access/README.md](./access/README.md)

### Access Example

The [access/example](./access/example) plugin automatically approves access requests based
on a user whitelist.  This is a good place to start if you are trying to understand
how to use the [`access`](./access) API.

Use `make access-example` to build the plugin and `./build/example configure` to print out
a sample configuration file.

### Slackbot

A basic slack plugin (WIP) can be found in [access/slackbot](./access/slackbot).
The plugin can be built with `make access-slackbot` and instructions for configuring the
plugin can be found in the plugin's [README](./access/slackbot/README.md).

## notes

This repository's `vendor` folder is just `gravitaional/teleport`'s `vendor`
folder "flattened" to include `gravitational/teleport` as a member, with the
exception of `nlopes/slack`, which is not a dependency of `teleport`.
