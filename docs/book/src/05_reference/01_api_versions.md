# API Versions

This page provides a detailed list of any changes between different API versions of CAPRKE2. It is intended to assist end users when upgrading from one version of the API to the next one.

### v1beta1 to v1beta2

#### Control Plane

In `v1beta1` the following fields were marked as `deprecated` and have been removed from `v1beta2`:
- `infrastructureRef` has been removed from `RKE2ControlPlane.spec`. Use `RKE2ControlPlane.spec.machineTemplate.spec.infrastructureRef` instead.
- `nodeDrainTimeout` has been removed from `RKE2ControlPlane.spec`. Use `RKE2ControlPlane.spec.machineTemplate.spec.deletion.nodeDrainTimeout` instead.

The following `v1beta1` fields have moved:
- `infrastructureRef` has moved from `RKE2ControlPlaneMachineTemplate` to `RKE2ControlPlaneMachineTemplateSpec.infrastructureRef` in `v1beta2`.
- `nodeDrainTimeout`, `nodeVolumeDetachTimeout` and `nodeDeletionTimeout` have moved under `RKE2ControlPlaneMachineTemplateSpec.deletion` in `v1beta2` and have been renamed to:
  - `deletion.nodeDrainTimeoutSeconds`
  - `deletion.nodeVolumeDetachTimeoutSeconds`
  - `deletion.nodeDeletionTimeoutSeconds`
  
  Note that these fields have changed their type to int32 and now expect a timeout expressed in seconds.

The `RKE2ControlPlaneMachineTemplate` object in `v1beta2` now includes a `spec` field which is required.

The following `v1beta1` `RKE2ControlPlaneStatus` fields have been moved under `RKE2ControlPlane.status.deprecated` in `v1beta2`:
- `conditions`
- `failureReason`
- `failureMessage`
- `updatedReplicas`
- `readyReplicas`
- `unavailableReplicas`

In `v1beta1`, status conditions are using `clusterv1beta1.Conditions`, which are CAPI-specific condition types, whereas in `v1beta2` they are using `metav1.Conditions`, inline with upstream CAPI. The benefit of using `metav1.Conditions` is that it provides a standard way of reporting status that is common across many Kubernetes resource types ([reference](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md)).

The following `RKE2ControlPlaneStatus` fields have been removed from `v1beta2`:
- `ready`
- `initialized`
- `dataSecretName`
- `failureReason`
- `failureMessage`
- `updatedReplicas`
- `unavailableReplicas`

An RKE2 cluster is considered initialized when `RKE2ControlPlaneStatus.initialization.controlPlaneInitialized` is set to `true`. 