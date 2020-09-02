# teleport-webhooks

`teleport-webhooks` is a teleport plugin that allows you to send a webhook to a
preconfigured URL with the access request meta information every time the access
request is created or updated (approved/denied) or deleted, and, optionally,
receive and handle HTTP requests to perform updates on access requests
(approvals/denials).

We created this plugin to make it easy for anyone to extend teleport with any
behavior they want, and learn how to create their own plugins when the existing
plugins don't work for their scenarios.

With `teleport-webhooks` you can:

- Roll your own authentication hooks system with a thin wrapper around your
  authorization management logic.
- Use requestbin to debug what information gets sent out on a request update
- Play around with Zapier or other similar systems.

## Zapier integration

## Prerequisites

## Getting started

## Configuration