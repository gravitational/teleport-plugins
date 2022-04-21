# Teleport Kubernetes Operator

This package implements [an operator for Kubernetes](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).
This operator is useful to bootstrap Teleport resources e.g. users and roles from [Kubernetes custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).
For more details, read the corresponding [RFD](https://github.com/gravitational/teleport-plugins/blob/master/rfd/0001-kubernetes-manager.md).

## Running

It's currently possible to run the operator using the sidecar approach.
Operator sidecar container is already integrated into [teleport-cluster](https://github.com/gravitational/teleport/tree/master/examples/chart/teleport-cluster) helm chart.

TODO: Neither the operator image nor the helm chart are released yet, so in order to try it out you should use [`marshall-lee/plugin-charts` branch](https://github.com/marshall-lee/teleport/tree/marshall-lee/plugin-charts).
See the instructions below on how to deploy the chart with a custom container image.

## Development

First of all, you need to install CRDs to your Kubernetes cluster.
There's a command for this that calls `install-crds` subcommand of the operator binary.

```
bash
% make install
go run . -- install-crds
{"level":"info","ts":1639510407.7697349,"logger":"setup","msg":"successfully installed CRD to the cluster","name":"identities.auth.teleport.dev","result":"created","old-operator-version":"","new-operator-version":"8.0.0","existing-crd-versions":[],"new-crd-versions":["v8"]}
{"level":"info","ts":1639510407.7702131,"logger":"setup","msg":"successfully installed CRD to the cluster","name":"roles.resources.teleport.dev","result":"created","old-operator-version":"","new-operator-version":"8.0.0","existing-crd-versions":[],"new-crd-versions":["v5"]}
{"level":"info","ts":1639510407.770255,"logger":"setup","msg":"successfully installed CRD to the cluster","name":"users.resources.teleport.dev","result":"created","old-operator-version":"","new-operator-version":"8.0.0","existing-crd-versions":[],"new-crd-versions":["v2"]}
```

Now you have some options how you can run the operator. The first and most convenient one is to run the operator on the host, outside of the Kubernetes cluster. In this mode you should also have Teleport running on the same host.

```bash
% make run
```

Of course, at some point you'll need to test your code in a real Kubernetes cluster. To do so, you need to build a custom container image and push it to some registry e.g. Docker Hub or [you can run the registry on a host machine](https://hub.docker.com/_/registry) to save some time without network round-trips.

To build the container image and push it to your registry:

```bash
% IMG=YOUR-REGISTRY/teleport-operator:10.0.0 make docker-build docker-push
```

To deploy the `teleport-cluster` helm chart with your custom operator image:

```bash
% helm install -n YOUR-NAMESPACE --set clusterName=YOUR-CLUSTER.local --set enterprise=true --set teleportVersionOverride=10.0.0 --set operatorImage=YOUR-REGISTRY/teleport-operator YOUR-DEPLOYMENT ~/code/go/teleport/examples/chart/teleport-cluster
```
