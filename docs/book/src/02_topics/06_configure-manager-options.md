## Supported Manager Options

At present, the following feature gates and flags are supported for configuration.

### Feature Gates

| Name            | Description                                           | Default | Env Variable     |
|-----------------|-------------------------------------------------------|---------|------------------|
| MachinePool     | for MachinePool functionality                         | true    | EXP_MACHINE_POOL |
| ClusterTopology | for ClusterClass and managed topologies functionality | true    | CLUSTER_TOPOLOGY |

### Flags

| Name        | Description                                                        | Default | Env Variable       |
|-------------|--------------------------------------------------------------------|---------|--------------------|
| concurrency | Number of core resources to process simultaneously                 | 10      | CONCURRENCY_NUMBER |

## Configuring Manager Options
In order to configure the manager options, it is required to patch the respective values in the 
`rke2-bootstrap-controller-manager` and `rke2-control-plane-controller-manager` deployments.

- Enable/Disable the feature flags:
> **Note:** Enabling/Disabling feature gates is supported only for `rke2-bootstrap-controller-manager`.
```shell
$ kubectl -n system edit deployment/rke2-bootstrap-controller-manager 

// Enable/disable available features by modifying args below.
    args:
      ...
    - "--feature-gates=MachinePool=true,ClusterResourceSet=true"
```

- Configure the concurrency options for manager:
> **Note:** Configuring manager options is supported for `rke2-bootstrap-controller-manager` as well as `rke2-control-plane-controller-manager`.

- Update `concurrency` arg:
```shell
$ kubectl -n system edit deployment/controller-manager

// Modify the value of  --concurrency  in args
   args:
      ...
    - "--concurrency=<required_value>"
```

The patch shall rollout the deployment, and should spin up new pods with updated values.