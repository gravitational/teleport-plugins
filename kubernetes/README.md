# Teleport Kubernetes Operator

This package implements [an operator for Kubernetes](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).
This operator is useful to bootstrap Teleport resources e.g. users and roles from [Kubernetes custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).
For more details, read the corresponding [RFD](https://github.com/gravitational/teleport-plugins/blob/master/rfd/0001-kubernetes-manager.md).

## Running

It's currently possible to run the operator using the sidecar approach.
Operator sidecar container is already integrated into [teleport-cluster](https://github.com/gravitational/teleport/tree/master/examples/chart/teleport-cluster) helm chart.

TODO: Neither the operator image nor the helm chart are released yet, so in order to try it out you should use `marco/plugins-teleport-operator-charts` [branch](https://github.com/gravitational/teleport/tree/marco/plugins-teleport-operator-charts).

## Development

We can set up the environment locally to speed up the testing.

We've created a little script which does the entire setup using minikube.
- starts minikube
- builds and tags the teleport operator into minikube's registry
- installs teleport-cluster helm chart with the operator and waits for the deployment to complete
- creates a tunnel so that we are able to interact with Teleport's UI
- opens the K8S dashboard (minikube dashboard)


Please note that we are deleting minikube's state, be careful when doing this as it might break your current "experiments".

```bash
#!/usr/bin/env bash
set -e # stop script when a command fails
set -o xtrace # print every line from now on

CLUSTER_NAME="teleport-cluster.teleport-cluster.svc.cluster.local"
NAMESPACE="teleport-cluster"
CLUSTER_SHORT="teleport-cluster"
CHART_LOCATION="$HOME/teleport/teleport/examples/chart/teleport-cluster" # gravitation/teleport at marco/plugins-teleport-operator-charts branch
IMG="teleport-operator:latest"
TELEPORT_VERSION="9.0.4"

minikube delete # clean up before starting
minikube start

# You must have the following tools available:
helm version
kubectl version
docker version

# build the teleport operator
pushd ~/teleport/teleport-plugins/kubernetes

# Build and tag our images to the registry
eval $(minikube docker-env --shell bash)
docker build --build-arg version=${TELEPORT_VERSION} -t ${IMG} -f ./Dockerfile ..

# install the CRDs
make install

# install the teleport-cluster chart with the sidecar operator
helm install --create-namespace -n ${NAMESPACE?} \
	--set clusterName=teleport-cluster.teleport-cluster.svc.cluster.local \
	--set teleportVersionOverride=${TELEPORT_VERSION} \
	--set operatorImage=${IMG} \
	--set operator=true \
	${CLUSTER_SHORT} ${CHART_LOCATION}

kubectl config set-context --current --namespace ${NAMESPACE?}
kubectl wait --for=condition=available deployment/${CLUSTER_SHORT} --timeout=2m

read -p "Run 'minikube tunnel' on another terminal and press enter"

PROXY_POD=$(kubectl get po -l app=teleport-cluster -o jsonpath='{.items[0].metadata.name}')
kubectl exec $PROXY_POD teleport -- tctl users add --roles=access,editor teleoperator
echo "open the following url (replace the invite id) and configure the user"
TP_CLUSTER_IP=$(kubectl get service teleport-cluster -o jsonpath='{ .status.loadBalancer.ingress[0].ip }')
echo "https://${TP_CLUSTER_IP}/web/invite/<id>"

minikube dashboard
```

Now you can interact with Kubernetes dashboard, Teleport UI and the usual k8s tools.
As an example, we can create a role using the following:

```yaml
apiVersion: "resources.teleport.dev/v5"
kind: Role
metadata:
  name: rolefromk
spec:
  allow:
    rules: []
```

```
$ kubectl apply -f <fila_above>
```
