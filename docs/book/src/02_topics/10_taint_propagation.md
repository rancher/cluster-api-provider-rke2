# Taint propagation

Set `spec.machineTemplate.spec.taints` on an `RKE2ControlPlane` to manage Node taints without replacing control-plane Machines. The same field is available under `spec.template.spec` on an `RKE2ControlPlaneTemplate`.

Enable `MachineTaintPropagation=true` in both the Cluster API and RKE2 control-plane controller managers. For installation with clusterctl, set `EXP_MACHINE_TAINT_PROPAGATION=true` before initializing the providers. This alpha feature is disabled by default.

For example, add this to an `RKE2ControlPlane` using the `v1beta2` API:

```yaml
spec:
  machineTemplate:
    spec:
      taints:
        - key: dedicated
          value: control-plane
          effect: NoSchedule
          propagation: Always
```

CAPRKE2 copies these taints to `Machine.spec.taints`. Cluster API propagates them to each Machine's Node according to the required `propagation` policy:

- `Always` adds and maintains the taint. Changing its value updates the Node; removing it from the template removes it from the Node.
- `OnInitialization` applies the taint only during the Node's first taint reconciliation. Later changes do not add it again or remove it from the Node.

Other Node taints are preserved. Taints are identified by their key and effect, and a template can contain up to 64 entries.

The bootstrap field `spec.agentConfig.nodeTaints` keeps its existing behavior. Use `spec.machineTemplate.spec.taints` for taints that need to change in place.
