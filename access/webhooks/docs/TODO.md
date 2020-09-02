# TODO

## Initial

- [x] Listen to all request types
- [x] Store and send out the cluster name
- [x] Send request state as string, not number

## Listener

- [ ] Add a response listener (http worker), only spawn it if !notifyOnly
- [ ] Write a minimal readme for read/write implementation

## Writeup

- [ ] Build a test flow with Google Calendar on Zapier

## Extend

Add config options:

- [ ] SendPending bool
- [ ] SendApproved bool
- [ ] SendDenied bool
- [ ] FormatOption string (default to json)
- [ ] Fields string id, user, roles, cluster_name, created, state, notify_only
