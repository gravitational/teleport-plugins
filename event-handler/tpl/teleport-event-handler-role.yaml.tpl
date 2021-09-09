kind: user
metadata:
  name: teleport-event-handler
spec:
  roles: ['teleport-event-handler']
version: v2
---
kind: role
metadata:
  name: teleport-event-handler
spec:
  allow:
    rules:
      - resources: ['event', 'session']
        verbs: ['list','read']
version: v4
