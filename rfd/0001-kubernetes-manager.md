---
authors: Vladimir Kochnev (marshall-lee@evilmartians.com)
state: draft
---

# RFD 1 - Kubernetes manager

## What

Teleport Kubernetes Manager is a [controller for Kubernetes](https://kubernetes.io/docs/concepts/architecture/controller/) serving Teleport resources defined as [Kubernetes custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).

## Why

Automated bootstrap of Teleport data is not an easy task, especially when it's running in Kubernetes. One can use `tctl` from the Teleport CLI toolset to bootstrap users/roles but integrate it into something like [Helm charts](https://helm.sh/docs/topics/charts/) quickly becomes a pain. In many scenarios, it's far more convenient to have the Teleport data defined the same way as the Kubernetes resources like Deployments and Services, in a format very similar to Teleport's own format.

## Details

### Implementation

For the task, the [Kubebuilder](https://kubebuilder.io/) framework should be used. It has great code generation capabilities and gives a complete setup to develop & deploy the manager.

### Teleport Auth server discovery and authentication

To write something into Teleport, the manager needs to know what server to connect to and how. There're many options on how to discover Teleport instances in the cluster but for now, let's start with the simplest one - run manager as a sidecar container in the same pod as the Teleport auth server. Containers in the pod can share the volumes, so the manager container could have access to `/var/lib/teleport/proc` database, which contains administrator credentials. This is the same way how `tctl` utility works - it doesn't require any authentication when you run it on the same machine where Teleport is running.

### Resource definitions

Custom resource definitions of Teleport resource look like this:

```yaml
apiVersion: resources.goteleport.com/v2
kind: Role
metadata:
  name: folks
  namespace: teleport
spec:
  allow:
    request:
      roles: ["admin"]
---
apiVersion: resources.goteleport.com/v4
kind: User
metadata:
  name: human-bean
  namespace: teleport
spec:
  roles: ["folks"]
```

Manager process watches for custom resource events. Once a custom resource is created/updated, the manager's watcher acknowledges it and upserts a resource into Teleport.

The name of the Kubernetes custom resource is the name of the resource in Teleport. Also, every Kubernetes custom resource must have a `spec` field that has the same schema as a `spec` field of a resource in Teleport.

Deletion of resources is being handled using [resource finalizers](https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/). Every acknowledged resource gets a finalizer named `"resources.goteleport.com/delete"` so when the deletion happens, Kubernetes doesn't actually delete it but sets a deletion timestamp. While the finalizer list is non-empty, the manager can still access the resource's fields. Deletion of the Kubernetes object leads to deletion of the associated resource in Teleport. Once a deletion in Teleport succeeds, the manager removes a finalizer from the Kubernetes object so it can be garbage-collected.

### Identity definitions

To run some plugin in the cluster, the credentials are required. Normally, one can get the credentials credentials using `tctl auth sign` command or using `tsh login`. To automate the credentials generation, the `Identity` resource is introduced. The identity definition looks like this:

```yaml
apiVersion: control.goteleport.com/v8
kind: Identity
metadata:
  name: access-slack
spec:
  username: access-slack
  secretName: access-slack-secret
```

Here, the `username` is a Teleport user name, and `secretName` is a name of a secret resource where the manager will write credentials. Once the `Identity` object is created in the Kubernetes, the secret object with credentials contents will be written in the same namespace where the identity object resides.

### Limit the scope

When running the manager as a sidecar container, the question arises: what objects is this particular instance of the manager responsible for and what objects not? There could be multiple Teleport deployments in the same cluster, and each deployment might have its manager sidecar container. So the question is how to limit the scope of objects observed.

The first option is to limit by the namespace, and the second is to limit by label selectors. Both of them must be present in the manager to be very specific on what Kubernetes objects to watch.

Configuration example:

```yaml
scope:
  namespace: some-namespace
  labelSelector:
    matchExpressions:
    - key: app
      operator: In
      values: ["my-teleport"]
```

### API Groups and Versions

`Identity` resource resides in an API group called `control.goteleport.com`, and all the Teleport resources reside in a group called `resources.goteleport.com`. Two different groups are used for a reason:

- API Versions of `instances.goteleport.com` start from `v8` because the Teleport version is 8.x at the moment of writing this document.
- API Versions of `resources.goteleport.com` are all different because they're tied to Teleport resource versions.

### Generating Custom Resource Definitions

To understand a custom resource, Kubernetes needs [its definition & OpenAPI v3.0 schema](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).

In standard scenarios, the Kubebuilder can generate CRDs right from the Go type definitions. However, the generator breaks when something like `types.UserSpecV2` or `types.RoleSpecV4` is used as a `Spec` field. Type definitions from `types.pb.go` are initially written for Protobuf and can't be directly re-used to generate [OpenAPI V3.0 schema](https://swagger.io/specification/).

To generate all the `resources.goteleport.com` APIs, one should implement a generator similar to the one done for [Terraform provider](https://github.com/gravitational/teleport-plugins/tree/master/terraform).
