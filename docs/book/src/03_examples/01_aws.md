# Cluster API AWS Infrastructure Provider

## Installing the AWS provider 

Refer to the [Cluster API book](https://cluster-api.sigs.k8s.io/user/quick-start#initialization-for-common-providers) for configuring AWS credentials and setting up the AWS infrastructure provider. This involves using `clusterawsadm` to create the necessary IAM roles and to generate the `AWS_B64ENCODED_CREDENTIALS` environment variable.

The next step is to run the clusterctl init command (make sure to provide valid AWS Credential using the `AWS_B64ENCODED_CREDENTIALS` environment variable):

CAPRKE2 can also be deployed with `clusterctl`

```bash
clusterctl init --bootstrap rke2 --control-plane rke2 --infrastructure aws
```

## Create a workload cluster

Before creating a workload clusters, it is required to build an AMI for the RKE2 version that is going to be installed on the cluster. You can follow the steps in the [image-builder README](https://github.com/rancher/cluster-api-provider-rke2/tree/main/image-builder#aws) to build the AMI.

You will need to set the following environment variables:

```bash
export CONTROL_PLANE_MACHINE_COUNT=3
export WORKER_MACHINE_COUNT=1
export RKE2_VERSION=v1.30.2+rke2r1
export AWS_NODE_MACHINE_TYPE=t3a.large
export AWS_CONTROL_PLANE_MACHINE_TYPE=t3a.large 
export AWS_SSH_KEY_NAME="aws-ssh-key"
export AWS_REGION="aws-region"
export AWS_AMI_ID="ami-id"
```

Now, we can generate the YAML files from the templates using `clusterctl generate yaml` command:

```bash
clusterctl generate cluster --from https://github.com/rancher/cluster-api-provider-rke2/blob/main/examples/templates/aws/cluster-template.yaml -n example-aws rke2-aws > aws-rke2-clusterctl.yaml
```

> NOTE: The template used above is tailored for air-gapped environments. For non air-gapped environments, you should adjust the resulting YAML file by setting `airGapped: false` in the relevant`RKE2ConfigTemplate` and `RKE2ControlPlane` resources.

After examining the resulting YAML file, you can apply to the management cluster using :

```bash
kubectl apply -f aws-rke2-clusterctl.yaml
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

## Ignition based bootstrap

**Note**: `ignition` template is currently outdated.

Make sure that `BootstrapFormatIgnition` feature gate is enable for CAPA manager, you can do it
by changing flag in the CAPA manager deployment:

```yaml
containers:
- args:
  - --feature-gates=EKS=true,EKSEnableIAM=false,EKSAllowAddRoles=false,EKSFargate=false,MachinePool=false,EventBridgeInstanceState=false,AutoControllerIdentityCreator=true,BootstrapFormatIgnition=true,ExternalResourceGC=false
  ...
  name: manager
```
or by setting the following environment variable before installing CAPA with `clusterctl`:

```bash
export BOOTSTRAP_FORMAT_IGNITION=true
```

For the Ignition based bootstrap, you will also need to set the following environment variables:

```bash
export AWS_S3_BUCKET_NAME=<YOUR_AWS_S3_BUCKET_NAME>
```

Now you can generate manifests from the cluster template:

```bash
clusterctl generate cluster --from https://github.com/rancher/cluster-api-provider-rke2/blob/main/examples/templates/aws/cluster-template-ignition.yaml -n example-aws rke2-aws > aws-rke2-clusterctl.yaml
```
