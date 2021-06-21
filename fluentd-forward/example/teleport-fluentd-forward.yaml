kind: user
metadata:
  name: teleport-fluentd-forward
spec:
  roles: ['teleport-fluentd-forward']
version: v2
---
kind: role
metadata:
  name: teleport-fluentd-forward
spec:
  allow:
    rules:
      - resources: ['events']
        verbs: ['list','read']
version: v4
