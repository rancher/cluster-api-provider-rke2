# Excluding control plane Nodes from external load balancers

## Overview

The `rke2.controlplane.cluster.x-k8s.io/load-balancer-exclusion: "true"` annotation can be set on `RKE2ControlPlanes` in order to apply the [nodes.kubernetes.io/exclude-from-external-load-balancers](https://kubernetes.io/docs/reference/labels-annotations-taints/#node-kubernetes-io-exclude-from-external-load-balancers) label on Nodes, during the pre-drain CAPI Machine phase, just before the Machine is deleted.  

This allows external load balancers that honor this label, to have enough time to stop advertising the Node, before etcd membership is lost, or the Machine is shut down.

## Using Load Balancer Exclusion

The annotation can be set on `RKE2ControlPlanes`.  
Upon setting the annotation, all control plane Machines will be marked with `pre-drain.delete.hook.machine.cluster.x-k8s.io/rke2-lb-exclusion` annotation.  

Note that it is possible to remove the annotation from the `RKE2ControlPlane`, triggering a cleanup of the related `pre-drain` hook annotation on all Machines.

Example:

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
kind: RKE2ControlPlane
metadata:
  name: my-control-plane
  annotations:
    rke2.controlplane.cluster.x-k8s.io/load-balancer-exclusion: "true"
```
