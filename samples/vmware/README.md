# Example manifests

This config includes a kubevip loadbalancer on the controlplane nodes. The VIP of the loadbalancer for the Kubernetes API is set by the CABPR_CONTROLPLANE_ENDPOINT variable.

Usage: 

export environmental variables below

```
export CABPR_NAMESPACE=example
export CABPR_CLUSTER_NAME=rke2
export CABPR_CP_REPLICAS=3
export CABPR_WK_REPLICAS=2
export KUBERNETES_VERSION=v1.24.6
export RKE2_VERSION=v1.24.6+rke2r1
export CABPR_CONTROLPLANE_ENDPOINT=192.168.1.100

export CABPR_VCENTER_HOSTNAME=vcenter.example.com
export CABPR_VCENTER_USERNAME=admin
export CABPR_VCENTER_PASSWORD=password
export CABPR_VCENTER_DATACENTER=datacenter
export CABPR_VCENTER_NETWORK=vmnetwork
export CABPR_VCENTER_THUMBPRINT=
export CABPR_VCENTER_DATASTORE=datastore
export CABPR_VCENTER_DISKSIZE=25
export CABPR_VCENTER_FOLDER=vm-folder
export CABPR_VCENTER_RESOURCEPOOL="*/Resources/resoucrepool"
export CABPR_VCENTER_VM_VPCU=2
export CABPR_VCENTER_VM_MEMORY=4096
export CABPR_VCENTER_VM_TEMPLATE=template
```

Create the namespace first.

run:
```shell
envsubt < namespace.yaml | kubectl apply -f -
envsubt < *.yaml | kubectl apply -f -
```
