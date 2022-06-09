# Teleport Kubernetes Operator

This package implements [an operator for Kubernetes](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).
This operator is useful to manage Teleport resources e.g. users and roles from [Kubernetes custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)

For more details, read the corresponding [RFD](https://github.com/gravitational/teleport-plugins/blob/master/rfd/0001-kubernetes-manager.md).

## Architecture
Teleport Operator is a K8S operator based on `operator-sdk`.
The operator lives right beside the Teleport server.

### Setup
When the operator starts it:
- grabs the leader lock, to ensure we only have one operator
- ensures the exclusive role and user exist
- creates an identity file using that user
- starts a new client, using that identity file and manages the resources

After this point, the operator only listens to modifications in the K8S cluster.

### Reconciliation
When something changes (either by a `kubectl apply` or from any other source), we start the reconciliation.

First, we try to identify the operation type: deletion or creation/modification.

If it's a deletion and we have our own finalizer, we remove the object in Teleport and remove the finalizer.
K8S will auto-remove the object when it gets 0 finalizers.

If it's a creation/modification:
- we add the finalizer, if it's not there already
- we lookup the object in Teleport side, and if it's not there, we set the Origin label to `kubernetes`
- we either create or update the resource in Teleport

The first two steps above may change the object's state in K8S. If they do, we update and finish the cycle - we'll receive another reconciliation request with the new state.

### Diagram
```
                   teleport cluster
+--------------------------------------------------------+
|                                                        |         +------+
|                                                        |         |      |
|     teleport                                           |         |      |
| +---------------------------------+                    |         |      |
| |                                 |                    |         +-+----+
| |                                 |                    |           |
| |                            +----+                    |           | kubectl apply -f
| |  +-------------+           |gRPC|<--+                |           |
| |  |/etc/teleport|           +----+   |                |           |
| |  +^------------+                |   |                |           |
| |   |                             |   |                |           |
| |   |   +-----------------+       |   | Manage         |           |
| |   |   |/var/lib/teleport|       |   | Resources      |           |
| |   |   +^----------------+       |   |                |           |     K8S
| |   |    |                        |   |                |      +----v----------------+
| +---+----+------------------------+   |                |      |                     |
|     |    |                            |                |      |    +--------------+ |
|     |    |                            |                |      |    |              | |
|     |    |   operator                 |                |      |    | Reconciler   | |
| +---+----+----------------------------+--------+       |      |    | Loop         | |
| |   |    |                            |        |       |      |    |              | |
| |  ++----+-+                    +-----v----+   |       |      |    |              | |
| |  |tctl   <--------------------> teleport <---+-------+------+---->              | |
| |  +-------+    Setup U&R       | operator |   |       |      |    |              | |
| |             Create Identity   +----------+   |       |      |    |              | |
| |                                              |       |      |    +--------------+ |
| |                                              |       |      |                     |
| +----------------------------------------------+       |      +---------------------+
|                                                        |
|                                                        |
|                                                        |
+--------------------------------------------------------+
```

## Running

### Requirementes

#### K8S cluster
You can use `minikube` if you don't have a cluster yet.

#### Operator's docker image
You can obtain the docker image by building it yourself

We're planning to upload the image to quay.io, but for now you have to build it yourself with the following command: `make docker-build-kubernetes`

This creates a `teleport-operator:latest` image that we'll be using later on to setup de Operator.

#### HELM chart from Teleport with the operator
We are re-using the Teleport Cluster Helm chart but modifying to start the operator using the image above.

This change is not in master, so you'll need to checkout the HELM charts from this branch
`marco/plugins-teleport-operator-charts` (https://github.com/gravitational/teleport/pull/12144)

#### Other tools
We also need the following tools: `helm`, `kubectl` and `docker`

### Running the operator

Start `minikube` with `minikube start`.

In order to build the Operator's image, we must use the docker daemon from the `minikube`:
`eval $(minikube docker-env --shell bash)`.

Change to the Project's root directory (`teleport-plguins`) and build the docker image:
`make docker-build-kubernetes`.

Meanwhile (or right after) we need to install the CRDs into the cluster.

You can do so by issuing `make kubernetes-install`

You are now ready to install the Teleport Cluster with the Teleport Operator:

Before executing, set the `TELEPORT_PROJECT` to the full path to your teleport's project checked out at `marco/plugins-teleport-operator-charts`
```bash
$ helm upgrade --install --create-namespace -n teleport-cluster \
	--set clusterName=teleport-cluster.teleport-cluster.svc.cluster.local \
	--set teleportVersionOverride="9.2.4" \
	--set operatorImage="teleport-operator:latest" \
	--set operator=true \
	teleport-cluster ${TELEPORT_PROJECT}/examples/chart/teleport-cluster

$ kubectl config set-context --current --namespace ${NAMESPACE?}

```

Now let's wait for the deployment to finish:
`kubectl wait --for=condition=available deployment/${CLUSTER_SHORT} --timeout=2m`

If it doesn't, check the errors.

Now, we want access to two configuration tools using a Web UI: K8S UI and Teleport UI.

First, let's create a tunnel using minikube: `minikube tunnel` (this command runs is foreground, open another terminal for the remaining commands).

Now, let's create a new Teleport User and login in the web UI:
```bash
PROXY_POD=$(kubectl get po -l app=teleport-cluster -o jsonpath='{.items[0].metadata.name}')
kubectl exec $PROXY_POD teleport -- tctl users add --roles=access,editor teleoperator
echo "open following url (replace the invite id) and configure the user"
TP_CLUSTER_IP=$(kubectl get service teleport-cluster -o jsonpath='{ .status.loadBalancer.ingress[0].ip }')
echo "https://${TP_CLUSTER_IP}/web/invite/<id>"
```

As for the K8S UI, you only need to run `minikube dashboard`

After this, you should be able to manage users and roles using, for example, `kubectl`

As an example, create the following file (`roles.yaml`) and then apply it:
```yaml
apiVersion: "resources.teleport.dev/v5"
kind: Role
metadata:
  name: myrole
spec:
  allow:
    logins: ["root"]
    kubernetes_groups: ["edit"]
    node_labels:
      dev: ["dev", "dev2"]
      type: [ "compute", "x" ]
```

`$ kubcetl apply -f roles.yaml`

And now check if the role was created in Teleport and K8S (`teleport-cluster` namespace)
