## Access API

## Overview

The Access API allows programmatic management (approval/denial) of Access
Requests.

## GRPC API

The GRPC API, defined in `teleport/lib/auth/proto/auth.proto`, includes a
handful of methods related to the `AccessRequest` resource. Most important for
the purposes of _managing_ access requests are the `WatchAccessRequests` and
`SetAccessRequestState` methods:

```grpc
rpc WatchAccessRequests(services.AccessRequestFilter) returns (stream services.AccessRequestV1);
rpc SetAccessRequestState(RequestStateSetter) returns (google.protobuf.Empty);
```

These methods allow integrations to be notified when requests are created, and
approve/deny said requests based on external factors (e.g. approval, calendar,
etc...).

## Authentication

In order to interact with the Access Request API, you will need to provision
appropriate TLS certificates. In order to provision certificates, you will need
to create an appropriate user with appropriate permissions:

```bash
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
        verbs: ['list','read','update'] # Note that you can not provide the update permission to the Slack plugin in notification_only mode.
    # teleport currently refuses to issue certs for a user with 0 logins,
    # this restriction may be lifted in future versions.
    logins: ['access-plugin']
version: v3
EOF
# ...
$ tctl create rscs.yaml
# ...
$ tctl auth sign --format=tls --user=access-plugin --out=auth
# ...
```

The above sequence should result in three PEM encoded files being generated:
`auth.crt`, `auth.key`, and `auth.cas` (certificate, private key, and CA certs
respectively).

_Note:_ by default, `tctl auth sign` produces certificates with a relatively
short lifetime. For production deployments, the `--ttl` flag can be used to
ensure a more _practical_ certificate lifetime.

## The `access` Package

The `access` package (defined in this directory) provides a thin wrapper around
the GRPC API that abstracts over some implementation details. If you are writing
an integration in golang, this is probably what you want.

See the [example](./example) directory for an example plugin implemented upon
the `access` package.
