# Development

The following instructions are for development purposes.

1. Clone the [Cluster API Repo](https://github.com/kubernetes-sigs/cluster-api) into the **GOPATH**

    > **Why clone into the GOPATH?** There have been historic issues with code generation tools when they are run outside the go path

2. Fork the [Cluster API Provider RKE2](https://github.com/rancher/cluster-api-provider-rke2) repo
3. Clone your new repo into the **GOPATH** (i.e. `~/go/src/github.com/yourname/cluster-api-provider-rke2`)
4. Ensure **Tilt** and **kind** are installed
5. Create a `tilt-settings.json` file in the root of the directory where you cloned the Cluster API repo in step 1.
6. Add the following contents to the file (replace `/path/to/clone/of/` with appropriate file path and "yourname" with 
   your github account name):

    ```json
    {
        "default_registry": "ghcr.io/yourname",
        "provider_repos": ["/path/to/clone/of/github.com/yourname/cluster-api-provider-rke2"],
        "enable_providers": ["docker", "rke2-bootstrap", "rke2-control-plane"],
        "kustomize_substitutions": {
            "EXP_MACHINE_POOL": "true",
            "EXP_CLUSTER_RESOURCE_SET": "true"
        },
        "extra_args": {
            "rke2-bootstrap": ["--v=4"],
            "rke2-control-plane": ["--v=4"],
            "core": ["--v=4"]
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

    > NOTE: if you are using Docker Desktop v4.13 or above then you will encounter issues from here. Until 
    > a permanent solution is found its recommended you use v4.12

9. Run the following command to create a local kind cluster:

    ```bash
    kind create cluster --config kind-cluster-with-extramounts.yaml
    ```

10. Now start tilt by running the following command in the directory where you cloned Cluster API repo in step 1:

    ```bash
    tilt up
    ```

11. Press the **space** key to see the Tilt web ui and check that everything goes green.

### Troubleshooting

Doing `tilt up` should install a bunch of resources in the underlying kind cluster. If you don't see anything there, 
run `tilt logs` in a separate terminal without stopping the `tilt up` command that you originally started.

#### Common Issues

1. Make sure you run the `kind` and `tilt` commands mentioned above from correct directory.
2. A `go mod vendor` might be required in your clone of CAPI repo. `tilt logs` should make this obvious.