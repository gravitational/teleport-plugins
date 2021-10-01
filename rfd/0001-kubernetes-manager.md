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

### Instance definitions

To write something into Teleport, the manager needs to know what server to connect to and how. So it needs Teleport Auth/Proxy/Tunnel address and credentials to be recognized. 

Instance definitions look like this:

```yaml
apiVersion: instances.goteleport.com/v7
kind: Instance
metadata:
  name: some-teleport-instance
  namespace: teleport
spec:
  addr: some-teleport.svc.cluster.local:3080
  secretName: some-creds
```

Credentials are stored separately from the instance definition in a secret defined in `secretName` field. An instance's secret must reside in the same namespace as an instance resource itself.

Credentials data contains some private keys, and it's a common practice to store such things in a Kubernetes secret. Such separation also plays nice with Kubernetes RBAC — one would want to give access to read an instances list and introspect their status but don't give access to their secrets.

Then one can create a credentials secret with something like that:

```bash
tctl auth sign --user=kubernetes-manager --ttl=8760h --overwrite --format=file -o /tmp/kubernetes-manager-identity
kubectl -n teleport create secret generic some-creds --from-file=identity=/tmp/kubernetes-manager-identity
rm -f /tmp/kubernetes-manager-identity
```

Secret in Kubernetes is a key-value object, and the only supported key, for now, is `"identity"` that stores Teleport connection credentials.

In the future, the process of identity derivation might be simplified by adding to `tctl auth sign` support of writing an identity directly to Kubernetes instead of a temporary file.

### Resource definitions

Custom resource definitions of Teleport resource look like this:

```yaml
apiVersion: resources.goteleport.com/v2
kind: Role
metadata:
  name: folks
  namespace: teleport
instanceName: some-instance
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
instanceName: some-instance
spec:
  roles: ["folks"]
```

Manager process watches for custom resource events. Once a custom resource is created/updated, the manager's watcher acknowledges it and upserts a resource into Teleport. Every custom resource must have the `instanceName` field useful to look up an instance resource in order to know what address to connect to. Instance resource and its associated secret must reside in the same namespace as the custom resource.

The name of the Kubernetes custom resource is the name of the resource in Teleport. Also, every Kubernetes custom resource must have a `spec` field that has the same schema as a `spec` field of a resource in Teleport.

Deletion of resources is being handled using [resource finalizers](https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/). Every acknowledged resource gets a finalizer named `"resources.goteleport.com/delete"` so when the deletion happens, Kubernetes doesn't actually delete it but sets a deletion timestamp. While the finalizer list is non-empty, the manager can still access the resource's fields, `instanceName` in particular. Deletion of the Kubernetes object leads to deletion of the associated resource in Teleport. Once a deletion in Teleport succeeds, the manager removes a finalizer from the Kubernetes object so it can be garbage-collected.

### API Groups and Versions

`Instance` resource resides in an API group called `instances.goteleport.com`, and all the Teleport resources reside in a group called `resources.goteleport.com`. Two different groups are used for a reason:

- API Versions of `instances.goteleport.com` start from `v7` because the Teleport version is 7.x at the moment of writing this document.
- API Versions of `resources.goteleport.com` are all different because they're tied to Teleport resource versions.

### Generating Custom Resource Definitions

To understand a custom resource, Kubernetes needs [its definition & OpenAPI v3.0 schema](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).

In standard scenarios, the Kubebuilder can generate CRDs right from the Go type definitions. However, the generator breaks when something like `types.UserSpecV2` or `types.RoleSpecV4` is used as a `Spec` field. Type definitions from `types.pb.go` are initially written for Protobuf and can't be directly re-used to generate [OpenAPI V3.0 schema](https://swagger.io/specification/).

To generate all the `resources.goteleport.com` APIs, one should implement a generator similar to the one done for [Terraform provider](https://github.com/gravitational/teleport-plugins/tree/master/terraform).
