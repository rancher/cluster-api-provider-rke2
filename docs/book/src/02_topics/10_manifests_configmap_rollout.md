# Manifests ConfigMap and control plane rollout

## Overview

`RKE2ControlPlane.spec.manifestsConfigMapReference` points to a `ConfigMap`.
Each data entry in the `ConfigMap` is copied to a folder on the control
plane nodes. RKE2 scans this folder and deploys the manifests it finds.

Changes to the `ConfigMap` do not trigger a control plane rollout. This
applies to both the `ConfigMap` content and the `manifestsConfigMapReference`
value itself.

## Behavior

New content from the `ConfigMap` reaches a control plane Machine only when
RKE2 provisions that Machine. This happens in 2 cases:

1. You create a new control plane Machine.
2. An unrelated change, for example a Kubernetes version upgrade, triggers a
   rollout of existing control plane Machines.

If you edit the `ConfigMap` and no rollout happens for another reason,
existing control plane Machines keep their current manifests. Only new
Machines get the updated content.

## Manifests that must roll out with the ConfigMap

Use `spec.files` on the `RKE2ControlPlane` if you need a manifest to trigger a
rollout on existing control plane Machines. Changes to `spec.files` do
trigger a rollout.

Example:

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1beta2
kind: RKE2ControlPlane
metadata:
  name: my-control-plane
spec:
  files:
    - path: /var/lib/rancher/rke2/server/manifests/my-manifest.yaml
      content: |
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: example
      owner: "root:root"
      permissions: "0644"
```
