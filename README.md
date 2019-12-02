# teleport-plugins

Set of plugins for Teleport


## access-request plugins

The [`access`](./access) package exposes a simple API for managing access requests
which can be used for writing plugins.  More info can be found in
[access/README.md](./access/README.md)

A basic slack plugin (WIP) can be found in [`access/slackbot`](./access/slackbot).
The plugin can be built with `make slackbot` and instructions for configuring the
plugin can be found in the plugin's [README](./access/slackbot/README.md).
