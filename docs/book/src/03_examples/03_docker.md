# Cluster API Docker Infrastructure Provider

This page focuses on using the RKE2 provider with the Docker Infrastructure provider.

## Setting up the Management Cluster

Make sure you set up a Management Cluster to use with Cluster API, you can follow instructions from the Cluster API [book](https://cluster-api.sigs.k8s.io/user/quick-start.html).

## Create a workload cluster

Before creating a workload clusters, it is required to set the following environment variables:

```bash
export CONTROL_PLANE_MACHINE_COUNT=3
export WORKER_MACHINE_COUNT=1
export RKE2_VERSION=v1.30.2+rke2r1
export KIND_IMAGE_VERSION=v1.30.0
```

Now, we can generate the YAML files from the templates using `clusterctl generate yaml` command:

```bash
clusterctl generate cluster --from https://github.com/rancher/cluster-api-provider-rke2/blob/main/examples/templates/docker/cluster-template.yaml -n example-docker rke2-docker > docker-rke2-clusterctl.yaml
```

After examining the result YAML file, you can apply to the management cluster using:

```bash
kubectl apply -f docker-rke2-clusterctl.yaml
```
