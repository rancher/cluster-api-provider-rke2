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
   > NOTE: You can also use your existing kind cluster by using the environment variable `CAPI_KIND_CLUSTER_NAME`.
10. Export `CLUSTER_TOPOLOGY=true` to enable support for managed topologies and ClusterClass.
11. Now start tilt by running the following command in the directory where you cloned Cluster API repo in step 1:

    ```bash
    tilt up
    ```

11. Press the **space** key to see the Tilt web ui and check that everything goes green.

## Attaching the debugger

This section explains how to attach debugger to the CAPRKE2 process running in a pod started using above steps. By
connecting a debugger to this process, you would be able to step through the code through your IDE. This guide
covers two popular IDEs - IntelliJ GoLand and VS Code.

On the Tilt web UI and confirm the port on which `caprke2_controller` is exposed. By default, it should be localhost:30002.

If you want to debug CAPRKE2 bootstrap controller, find where `capbpr_controller` is exposed. By default, it should be localhost:30001.

To learn about debugging Core CAPI Controller, refer to [the official doc](https://cluster-api.sigs.k8s.io/developer/core/tilt). 

### GoLand

1. Go to Run -> Edit Configurations.
2. In the dialog box that opens up, add a new configuration by clicking on `+` sign in top left and selecting 'Go
   Remote'.
3. Enter the 'Host' and 'Port' values based on where `caprke2_controller` is exposed.

You can setup a similar debugger for the Bootstrap controller.

To debug Bootstrap and Control Plane simultaneously, you can add a new configuration as before but select 'Compound' and then select the Go Remote setups for both controllers.

### VS Code

1. If you don't already have a `launch.json` setup, go to 'Run and Debug' in the Activity Bar and click on 'create a
   launch.json file'. Press the escape key; you'll be presented with the newly created `launch.json` file.
   Alternatively, create a `.vscode` directory in the root of your git repo and create a`launch.json` file in it.
2. Insert the following configuration in the file:
    ```json
    {
        "version": "0.2.0",
        "configurations": [
            {
                "name": "Connect to server",
                "type": "go",
                "request": "attach",
                "mode": "remote",
                "remotePath": "${workspaceFolder}",
                "port": 30002,
                "host": "127.0.0.1"
            }
        ]
    }
    ```
---
Insert a breakpoint, e.g., in the [`updateStatus`](https://github.com/rancher/cluster-api-provider-rke2/blob/5163e6233301262c5bcfebab58189aa278b1f51e/controlplane/internal/controllers/rke2controlplane_controller.go#L298) method which responsible for updating the status of the `RKE2ControlPlane`
resource and run the configuration. To check if you can step into the code, create a workload cluster by using the
example provided in the [documentation](../03_examples/03_docker.md). If things were configured correctly, code
execution would halt at the breakpoint, and you should be able to step through it.

## Troubleshooting

Doing `tilt up` should install a bunch of resources in the underlying kind cluster. If you don't see anything there, 
run `tilt logs` in a separate terminal without stopping the `tilt up` command that you originally started.

### Common Issues

1. Make sure you run the `kind` and `tilt` commands mentioned above from correct directory.
2. A `go mod vendor` might be required in your clone of CAPI repo. `tilt logs` should make this obvious.

# Unit Testing
Unit tests use envtest, and to ensure you can run them locally, you need to install the envtest binary.

```shell
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
KUBEBUILDER_ASSETS=$(setup-envtest use -p path <k8s-version>)
```

Set the envvar `KUBEBUILDER_ASSETS` and its value in your code editor's test configuration.

# E2E Testing
To run or debug a test from your code editor, you would need to setup the configuration with some environment variables and arguments.
To run an e2e test, we use `make e2e-test` in the CI, we will replicate the same settings in our code editor configuration.

## Goland
Create a Run/Debug configuration with the following settings:
Environment variables:
1. LOCAL_IMAGES=/path/to/cluster-api-provider-rke2/out/images
2. CAPI_KUSTOMIZE_PATH=/path/to/cluster-api-provider-rke2/hack/tools/bin/kustomize-v5.5.0

Go tool arguments:
1. `--tags=e2e`

Program arguments:
1. `-e2e.artifacts-folder="/path/to/cluster-api-provider-rke2/_artifacts"`
2. `-e2e.config="/path/to/cluster-api-provider-rke2/test/e2e/config/e2e_conf.yaml"`
3. `-e2e.use-existing-cluster=false`
4. `-e2e.skip-resource-cleanup=false`