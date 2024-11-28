# Getting Started

Cluster API Provider RKE2 is compliant with the `clusterctl` contract, which means that `clusterctl` simplifies its deployment to the CAPI Management Cluster. In this Getting Started guide, we will be using the RKE2 Provider with the `docker` provider (also called `CAPD`).

## Prerequisites
- [clusterctl](https://cluster-api.sigs.k8s.io/user/quick-start#install-clusterctl) to handle the lifecycle of a Cluster API management cluster
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) to apply the workload cluster manifests that `clusterctl` generates
- [kind](https://kind.sigs.k8s.io/) and [docker](https://www.docker.com/) to create a local Cluster API management cluster

## Management Cluster

In order to use this provider, you need to have a management cluster available to you and have your current KUBECONFIG context set to talk to that cluster. If you do not have a cluster available to you, you can create a `kind` cluster. These are the steps needed to achieve that:
1. Ensure kind is installed (https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
2. Create a special `kind` configuration file if you intend to use the Docker infrastructure provider:

```bash
cat > kind-cluster-with-extramounts.yaml <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: capi-test
nodes:
- role: control-plane
  extraMounts:
    - hostPath: /var/run/docker.sock
      containerPath: /var/run/docker.sock
EOF
```

3. Run the following command to create a local kind cluster:

```bash
kind create cluster --config kind-cluster-with-extramounts.yaml
```

4. Check your newly created `kind` cluster :

```bash
kubectl cluster-info
```
and get a similar result to this:

```
Kubernetes control plane is running at https://127.0.0.1:40819
CoreDNS is running at https://127.0.0.1:40819/api/v1/namespaces/kube-system/services/kube-dns:dns/proxy

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```

## Setting up clusterctl

### CAPI >= v1.6.0

No additional steps are required and you can install the RKE2 provider with **clusterctl** directly:

```bash
clusterctl init --core cluster-api:v1.8.5 --bootstrap rke2:v0.9.0 --control-plane rke2:v0.9.0 --infrastructure docker:v1.8.5
```

Next, you can proceed to [creating a workload cluster](#create-a-workload-cluster).

### CAPI < v1.6.0

With CAPI & clusterctl versions less than v1.6.0 you need a specific configuration. To do this create a file called `clusterctl.yaml` in the `$HOME/.cluster-api` folder with the following content (substitute `${VERSION}` with a valid semver specification - e.g. v0.5.0 - from [releases](https://github.com/rancher/cluster-api-provider-rke2/releases)):

```yaml
providers:
  - name: "rke2"
    url: "https://github.com/rancher/cluster-api-provider-rke2/releases/${VERSION}/bootstrap-components.yaml"
    type: "BootstrapProvider"
  - name: "rke2"
    url: "https://github.com/rancher/cluster-api-provider-rke2/releases/${VERSION}/control-plane-components.yaml"
    type: "ControlPlaneProvider"
``` 
> NOTE: Due to some issue related to how `CAPD` creates Load Balancer healthchecks, it is necessary to use a fork of `CAPD` by providing in the above configuration file the following :

```yaml
  - name: "docker"
    url: "https://github.com/belgaied2/cluster-api/releases/v1.3.3-cabpr-fix/infrastructure-components.yaml"
    type: "InfrastructureProvider"
``` 

This configuration tells clusterctl where to look for provider manifests in order to deploy provider components in the management cluster. 

The next step is to run the `clusterctl init` command:

```bash
clusterctl init --bootstrap rke2 --control-plane rke2 --infrastructure docker:v1.3.3-cabpr-fix
```

This should output something similar to the following:

```
Fetching providers
Installing cert-manager Version="v1.10.1"
Waiting for cert-manager to be available...
Installing Provider="cluster-api" Version="v1.3.3" TargetNamespace="capi-system"
Installing Provider="bootstrap-rke2" Version="v0.1.0-alpha.1" TargetNamespace="rke2-bootstrap-system"
Installing Provider="control-plane-rke2" Version="v0.1.0-alpha.1" TargetNamespace="rke2-control-plane-system"

Your management cluster has been initialized successfully!

You can now create your first workload cluster by running the following:

  clusterctl generate cluster [name] --kubernetes-version [version] | kubectl apply -f -
```

## Create a workload cluster

There are some sample cluster templates available under the `examples` folder. This section assumes you are using CAPI v1.6.0 or higher.

For this `Getting Started` section, we will be using the `docker` samples available under `examples/docker/online-default` folder. This folder contains a YAML template file called `cluster-template.yaml` which contains environment variable placeholders which can be substituted using the [envsubst](https://github.com/a8m/envsubst/releases) tool. We will use `clusterctl` to generate the manifests from these template files.
Set the following environment variables:
- CABPR_NAMESPACE
- CLUSTER_NAME
- CABPR_CP_REPLICAS
- CABPR_WK_REPLICAS
- KUBERNETES_VERSION
- KIND_IMAGE_VERSION

for example:

```bash
export CABPR_NAMESPACE=example
export CLUSTER_NAME=capd-rke2-test
export CABPR_CP_REPLICAS=3
export CABPR_WK_REPLICAS=2
export KUBERNETES_VERSION=v1.30.3
export KIND_IMAGE_VERSION=v1.30.3
```

The next step is to substitue the values in the YAML using the following commands:

```bash
cd examples/docker/online-default
cat cluster-template.yaml | clusterctl generate yaml > rke2-docker-example.yaml
```

At this moment, you can take some time to study the resulting YAML, then you can apply it to the management cluster:

```bash
kubectl apply -f rke2-docker-example.yaml
```

and see the following output:

```
namespace/example created
cluster.cluster.x-k8s.io/capd-rke2-test created
dockercluster.infrastructure.cluster.x-k8s.io/capd-rke2-test created
rke2controlplane.controlplane.cluster.x-k8s.io/capd-rke2-test-control-plane created
dockermachinetemplate.infrastructure.cluster.x-k8s.io/controlplane created
machinedeployment.cluster.x-k8s.io/worker-md-0 created
dockermachinetemplate.infrastructure.cluster.x-k8s.io/worker created
rke2configtemplate.bootstrap.cluster.x-k8s.io/capd-rke2-test-agent created
configmap/capd-rke2-test-lb-config created
```

## Checking the workload cluster

After waiting several minutes, you can check the state of CAPI machines, by running the following command:

```bash
kubectl get machine -n example
```

and you should see output similar to the following:

```
NAME                                 CLUSTER          NODENAME                                 PROVIDERID                                          PHASE     AGE   VERSION
capd-rke2-test-control-plane-9fw9t   capd-rke2-test   capd-rke2-test-control-plane-9fw9t       docker:////capd-rke2-test-control-plane-9fw9t       Running   35m   v1.27.3+rke2r1
capd-rke2-test-control-plane-m2sdk   capd-rke2-test   capd-rke2-test-control-plane-m2sdk       docker:////capd-rke2-test-control-plane-m2sdk       Running   12m   v1.27.3+rke2r1
capd-rke2-test-control-plane-zk2xb   capd-rke2-test   capd-rke2-test-control-plane-zk2xb       docker:////capd-rke2-test-control-plane-zk2xb       Running   27m   v1.27.3+rke2r1
worker-md-0-fhxrw-crn5g              capd-rke2-test   capd-rke2-test-worker-md-0-fhxrw-crn5g   docker:////capd-rke2-test-worker-md-0-fhxrw-crn5g   Running   36m   v1.27.3+rke2r1
worker-md-0-fhxrw-qsk7n              capd-rke2-test   capd-rke2-test-worker-md-0-fhxrw-qsk7n   docker:////capd-rke2-test-worker-md-0-fhxrw-qsk7n   Running   36m   v1.27.3+rke2r1
```

## Accessing the workload cluster

Once cluster is fully provisioned, you can check its status with:

```bash
kubectl get cluster -n example
```

and see an output similar to this:

```
NAMESPACE   NAME             CLUSTERCLASS   PHASE         AGE   VERSION
example     capd-rke2-test                  Provisioned   31m
```

You can also get an “at glance” view of the cluster and its resources by running:

```bash
clusterctl describe cluster capd-rke2-test -n example
```

This should output similar to this:

```
NAME                                                            READY  SEVERITY  REASON  SINCE  MESSAGE                                                                         
Cluster/capd-rke2-test                                          True                     2m56s                                                                                   
├─ClusterInfrastructure - DockerCluster/capd-rke2-test          True                     31m                                                                                     
├─ControlPlane - RKE2ControlPlane/capd-rke2-test-control-plane  True                     2m56s                                                                                   
│ └─3 Machines...                                               True                     28m    See capd-rke2-test-control-plane-9fw9t, capd-rke2-test-control-plane-m2sdk, ...  
└─Workers                                                                                                                                                                        
  └─MachineDeployment/worker-md-0                               True                     15m                                                                                     
    └─2 Machines...                                             True                     25m    See worker-md-0-fhxrw-crn5g, worker-md-0-fhxrw-qsk7n
```

🎉 CONGRATULATIONS! 🎉 You created your first RKE2 cluster with CAPD as an infrastructure provider.

## Using ClusterClass for cluster creation

This provider supports using [ClusterClass](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20210526-cluster-class-and-managed-topologies.md), a Cluster API feature that implements an extra level of abstraction on top of the existing Cluster API functionality. The `ClusterClass` object is used to define a collection of template resources (control plane and machine deployment) which are used to generate one or more clusters of the same flavor.

If you are interested in leveraging this functionality, you can refer to the examples [here](https://github.com/rancher/cluster-api-provider-rke2/tree/main/examples/docker/clusterclass):
- [clusterclass-quick-start.yaml](https://github.com/rancher/cluster-api-provider-rke2/blob/main/examples/docker/clusterclass/clusterclass-quick-start.yaml): creates a sample `ClusterClass` and necessary resources.
- [rke2-sample.yaml](https://github.com/rancher/cluster-api-provider-rke2/blob/main/examples/docker/clusterclass/rke2-sample.yaml): creates a workload cluster using the `ClusterClass`.

As with other sample templates, you will need to set a number environment variables:
- CLUSTER_NAME
- CABPR_CP_REPLICAS
- CABPR_WK_REPLICAS
- KUBERNETES_VERSION
- KIND_IP

for example:

```bash
export CLUSTER_NAME=capd-rke2-clusterclass
export CABPR_CP_REPLICAS=3
export CABPR_WK_REPLICAS=2
export KUBERNETES_VERSION=v1.30.3
export KIND_IP=192.168.20.20
```

**Remember that, since we are using Kind, the value of `KIND_IP` must be an IP address in the range of the `kind` network.**
You can check the range Docker assigns to this network by inspecting it:

```bash
docker network inspect kind
```

The next step is to substitue the values in the YAML using the following commands:

```bash
cat clusterclass-quick-start.yaml | clusterctl generate yaml > clusterclass-example.yaml
```

At this moment, you can take some time to study the resulting YAML, then you can apply it to the management cluster:

```bash
kubectl apply -f clusterclass-example.yaml
```

This will create a new `ClusterClass` template that can be used to provision one or multiple workload clusters of the same flavor.
To do so, you can follow the same procedure and substitute the values in the YAML for the cluster definition:

```bash
cat rke2-sample.yaml | clusterctl generate yaml > rke2-clusterclass-example.yaml
```

And then apply the resulting YAML file to create a cluster from the existing `ClusterClass`.
```bash
kubectl apply -f rke2-clusterclass-example.yaml
```

## Known Issues

### When using CAPD  < v1.6.0 unmodified, Cluster creation is stuck after first node and API is not reachable

If you use `docker` as your infrastructure provider without any modification, Cluster creation will stall after provisioning the first node, and the API will not be available using the LB address. This is caused by Load Balancer configuration used in CAPD which is not compatible with RKE2. Therefore, it is necessary to use our own fork of `v1.3.3` by using a specific clusterctl configuration.