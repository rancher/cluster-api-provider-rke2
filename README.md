# Cluster API Provider RKE2

![GitHub](https://img.shields.io/github/license/rancher-sandbox/cluster-api-provider-rke)

------

 :exclamation: **This project is early in development**

## What is Cluster API Provider RKE2

The [Cluster API][cluster_api] brings declarative, Kubernetes-style APIs to cluster creation, configuration and management.

Cluster API Provider RKE2 is a combination of 2 provider types, a __Cluster API Control Plane Provider__ for provisioning Kubernetes control plane nodes and a __Cluster API Bootstrap Provider__ for bootstrapping Kubernetes on a machine where [RKE2](https://docs.rke2.io/) is used as the Kubernetes distro.

------

## Getting Started

These instructions are for development purposes initially and will be changed in the future for user facing instructions.

1. Clone the [Cluster API Repo](https://github.com/kubernetes-sigs/cluster-api) into the **GOPATH**

> **Why clone into the GOPATH?** There have been historic issues with code generation tools when they are run outside the go path

2. Fork the [Cluster API Provider RKE2](https://github.com/rancher-sandbox/cluster-api-provider-rke2) repo
3. Clone your new repo into the **GOPATH** (i.e. `~/go/src/github.com/myname/cluster-api-provider-rke2`)
4. Ensure **Tilt** and **kind** are installed
5. Create a `tilt-settings.json` file in the root of your forked/cloned `cluster-api` directory.
6. Add the following contents to the file (replace "yourname" with your github account name):

```json
{
    "default_registry": "ghcr.io/rancher-sandox",
    "provider_repos": ["../../github.com/rancher-sandbox/cluster-api-provider-rke2"],
    "enable_providers": ["docker", "rke2-bootstrap", "rke2-control-plane"],
    "kustomize_substitutions": {
        "EXP_MACHINE_POOL": "true",
        "EXP_CLUSTER_RESOURCE_SET": "true"
    },
    "extra_args": {
        "rke2-bootstrap": ["-zap-log-level=debug"],
        "rke2-control-plane": ["-zap-log-level=debug"],
        "core": ["-zap-log-level=debug"]
    },
    "debug": {
        "rke2-bootstrap": {
            "continue": true,
            "port": 30001
        },
        "rke2-control-plane": {
            "continue": true,
            "port": 30002
        }
    }
}
```

> NOTE: Until this [bug](https://github.com/kubernetes-sigs/cluster-api/pull/7482) merged in CAPI you will have to make the changes locally to your clone of CAPI.

7. Open another terminal (or pane) and go to the `cluster-api` directory.
8.  Run the following to create a configuration for kind:

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

> NOTE: if you are using Docker Desktop v4.13 or above then you will you will encounter issues from here. Until a permanent solution is found its recommended you use v4.12

9. Run the following command to create a local kind cluster:

```bash
kind create cluster --config kind-cluster-with-extramounts.yaml
```

10. Now start tilt by running the following:

```bash
tilt up
```

11. Press the **space** key to see the Tilt web ui and check that everything goes green.
