# Node Registration Methods

The provider supports multiple methods for registering a new node into the cluster.

## Usage

The method to use is specified on the **RKEControlPlane** within the **spec**. If no method is supplied then the default method of **internal-first** will be used.

> You cannot change the registration method after creation.

An example of using a different method:

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1alpha2
kind: RKE2ControlPlane
metadata:
  name: test1-control-plane
  namespace: default
spec:
  agentConfig:
    version: v1.26.4+rke2r1
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: DockerMachineTemplate
    name: controlplane
  nodeDrainTimeout: 2m
  replicas: 3
  serverConfig:
    cni: calico
  registrationMethod: "address"
  registrationAddress: "172.19.0.3"
```

## Registration Methods

### internal-first

For each CAPI `Machine` that is used for the control plane,  we take the **internal** ip address from `Machine.status.addresses` if it exists. If there is no **internal** ip for a machine then we will use an **external** address instead. For the ip address found for a machine then we add it to  `RKEControlPlane.status.availableServerIPs`.

The first IP address listed in `RKEControlPlane.status.availableServerIPs` is then used for the join.

### internal-only-ips

For each CAPI `Machine` that is used for the control plane,  we take the **internal** ip address from `Machine.status.addresses` if it exists and then we add it to  `RKEControlPlane.status.availableServerIPs`.

The first IP address listed in `RKEControlPlane.status.availableServerIPs` is then used for the join.

### external-only-ips

For each CAPI `Machine` that is used for the control plane,  we take the **external** ip address from `Machine.status.addresses` if it exists and then we add it to  `RKEControlPlane.status.availableServerIPs`.

The first IP address listed in `RKEControlPlane.status.availableServerIPs` is then used for the join.

### address

For this method you must supply an address in the control plane spec (i.e. `RKE2ControlPlane.spec.registrationAddress`). This address is then used for the join.

With this method its expected that you have a load balancer / VIP solution sitting in front of all the control plane machines and all the join requests will be routed via this.
