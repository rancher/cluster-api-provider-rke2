# Using CAPI Provider RKE2 with AWS
This README focuses on using the RKE2 provider with the AWS Infrastructure provider.

> NOTE: There is a known issue related to using RKE2 with AWS, which is related to security groups and the fact that RKE2 needs a specific port to be open to make node registration. The issue is documented [here](https://github.com/kubernetes-sigs/cluster-api-provider-aws/issues/392#issuecomment-1386975735) in the AWS Provider repo. However, as a temporary solution, you will need to use a [fork of the AWS Provider](https://github.com/belgaied2/cluster-api-provider-aws). 

## Setting up the Management Cluster
Make sure your set up a Management Cluster to use with Cluster API, example [here in the main README](https://github.com/rancher-sandbox/cluster-api-provider-rke2#management-cluster).

## Configuring `clusterctl` 
In order to use `clusterctl` to deploy an RKE2 cluster on AWS, you will need to configure the tool accordingly. First, make sure you have a folder `$HOME/.cluster-api`. Then, create (if it does not yet exist) a file called `clusterctl.yaml` inside that folder and edit it:

```bash 
vim ~/.cluster-api/clusterctl.yaml
```

And put the following configuration:
```yaml
providers:
  - name: "rke2"
    url: "https://github.com/rancher-sandbox/cluster-api-provider-rke2/releases/v0.1.0-alpha.1/bootstrap-components.yaml"
    type: "BootstrapProvider"
  - name: "rke2"
    url: "https://github.com/rancher-sandbox/cluster-api-provider-rke2/releases/v0.1.0-alpha.1/control-plane-components.yaml"
    type: "ControlPlaneProvider"
  - name: "aws"
    url: "https://github.com/belgaied2/cluster-api-provider-aws/releases/v2.0.2-cabpr-fix/infrastructure-components.yaml"
    type: "InfrastructureProvider"
```

The third provider configuration above is the one necessary to use the valid fork of the AWS Provider. This will need to be done until the issue with Security Groups is solved.

The next step is to run the clusterctl init command (make sure to provide valid AWS Credential using the `AWS_B64ENCODED_CREDENTIALS` environment variable):

```bash
export AWS_B64ENCODED_CREDENTIALS=<PUT_HERE_YOUR_BASE64_ENCODED_AWS_CREDENTIALS>
```

For the AWS external Cloud Provider, you will also need to set the ResourceSet experimental feature flag for CAPI:

```bash
export EXP_CLUSTER_RESOURCE_SET=true
```

Then: 

```bash
clusterctl init --bootstrap rke2 --control-plane rke2 --infrastructure aws:v2.0.2-cabpr-fix
```


This should output something similar to the following:

```
Fetching providers
Installing cert-manager Version="v1.10.1"
Waiting for cert-manager to be available...
Installing Provider="cluster-api" Version="v1.3.3" TargetNamespace="capi-system"
Installing Provider="bootstrap-rke2" Version="v0.1.0-alpha.1" TargetNamespace="rke2-bootstrap-system"
Installing Provider="control-plane-rke2" Version="v0.1.0-alpha.1" TargetNamespace="rke2-control-plane-system"
Installing Provider="infrastructure-aws" Version="v2.0.2-cabpr-fix" TargetNamespace="capa-system"

Your management cluster has been initialized successfully!

You can now create your first workload cluster by running the following:

  clusterctl generate cluster [name] --kubernetes-version [version] | kubectl apply -f -
```

## Create a workload cluster
The `internal` folder contains cluster templates to deploy an RKE2 cluster on AWS using the internal cloud provider (is DEPRECATED in favor of the external one), and the `external` folder contains the cluster templates to deploy a cluster with the external cloud provider.

We will use the `external` one for this guide.

You will need to set the following environment variables:
- CABPR_CP_REPLICAS
- CABPR_WK_REPLICAS
- KUBERNETES_VERSION
- AWS_NODE_MACHINE_TYPE
- AWS_CONTROL_PLANE_MACHINE_TYPE
- AWS_SSH_KEY_NAME
- AWS_REGION

Example:

```bash
export CABPR_CP_REPLICAS=3
export CABPR_WK_REPLICAS=2
export KUBERNETES_VERSION=v1.24.6
export AWS_NODE_MACHINE_TYPE=t3a.large
export AWS_CONTROL_PLANE_MACHINE_TYPE=t3a.large 
export AWS_SSH_KEY_NAME=<YOUR_AWS_PUBLIC_KEY_NAME>
export AWS_REGION=<YOUR_AWS_REGION>
```

Now, we can generate the YAML files from the templates using `clusterctl generate yaml` command:

```bash
clusterctl generate cluster --from https://github.com/rancher-sandbox/cluster-api-provider-rke2/blob/v0.1.0-alpha.1/samples/aws/external/cluster-template-external-cloud-provider.yaml -n example-aws rke2-aws > aws-rke2-clusterctl.yaml
```

After examining the result YAML file, you can apply to the management cluster using :

```bash
kubectl apply -f aws-rke2-clusterctl.yaml
```

You should see the following output:

```
namespace/example-aws created
cluster.cluster.x-k8s.io/rke2-aws created
awscluster.infrastructure.cluster.x-k8s.io/rke2-aws created
rke2controlplane.controlplane.cluster.x-k8s.io/rke2-aws-control-plane created
awsmachinetemplate.infrastructure.cluster.x-k8s.io/rke2-aws-control-plane created
machinedeployment.cluster.x-k8s.io/rke2-aws-md-0 created
awsmachinetemplate.infrastructure.cluster.x-k8s.io/rke2-aws-md-0 created
rke2configtemplate.bootstrap.cluster.x-k8s.io/rke2-aws-md-0 created
clusterresourceset.addons.cluster.x-k8s.io/crs-ccm created
clusterresourceset.addons.cluster.x-k8s.io/crs-csi created
configmap/cloud-controller-manager-addon created
configmap/aws-ebs-csi-driver-addon created
awsclustercontrolleridentity.infrastructure.cluster.x-k8s.io/default created
```

## Checking the workload cluster
After a while you should be able to check functionality of the workload cluster using `clusterctl`: 

```bash
clusterctl describe cluster -n example-aws rke2-aws
```

and once the cluster is provisioned, it should look similar to the following:

```
NAME                                                          READY  SEVERITY  REASON  SINCE  MESSAGE
Cluster/rke2-aws                                              True                     16m
├─ClusterInfrastructure - AWSCluster/rke2-aws                 True                     25m
├─ControlPlane - RKE2ControlPlane/rke2-aws-control-plane      True                     16m
│ └─3 Machines...                                             True                     19m    See rke2-aws-control-plane-8wsfm, rke2-aws-control-plane-qgwr7, ...
└─Workers
  └─MachineDeployment/rke2-aws-md-0                           True                     18m
    └─2 Machines...                                           True                     19m    See rke2-aws-md-0-6d47bf584d-g2ljz, rke2-aws-md-0-6d47bf584d-m9z8h
```