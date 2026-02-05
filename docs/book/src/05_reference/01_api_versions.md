# API Versions

This page provides a detailed list of any changes between different API versions of CAPRKE2. It is intended to assist end users when upgrading from one version of the API to the next one.

### v1beta1 to v1beta2

In `v1beta1` the following fields were marked as `deprecated` and have been removed from `v1beta2`:
- `infrastructureRef` has been removed from `RKE2ControlPlane.spec`. Use `RKE2ControlPlane.spec.machineTemplate.spec.infrastructureRef` instead.
- `nodeDrainTimeout` has been removed from `RKE2ControlPlane.spec`. Use `RKE2ControlPlane.spec.machineTemplate.spec.deletion.nodeDrainTimeout` instead.

The following `v1beta1` fields have moved:
- `infrastructureRef` has moved from `RKE2ControlPlaneMachineTemplate` to `RKE2ControlPlaneMachineTemplateSpec.infrastructureRef` in `v1beta2`.
- `nodeDrainTimeout`, `nodeVolumeDetachTimeout` and `nodeDeletionTimeout` have moved under `RKE2ControlPlaneMachineTemplateSpec.deletion` in `v1beta2`.

The `RKE2ControlPlaneMachineTemplate` object in `v1beta2` now includes a `spec` field which is required.

The following `v1beta1` `RKE2ControlPlaneStatus` fields have been moved under `RKE2ControlPlane.status.deprecated` in `v1beta2`:
- `conditions`
- `failureReason`
- `failureMessage`
- `updatedReplicas`
- `readyReplicas`
- `unavailableReplicas`

In `v1beta1`, status conditions are using `clusterv1beta1.Conditions` whereas in `v1beta2` they are using `metav1.Conditions`, inline with upstream CAPI.

The following `RKE2ControlPlaneStatus` fields have been removed from `v1beta2`:
- `ready`
- `initialized`
- `dataSecretName`
- `failureReason`
- `failureMessage`
- `updatedReplicas`
- `unavailableReplicas`

An RKE2 cluster is considered initialized when `RKE2ControlPlaneStatus.initialization.controlPlaneInitialized` is set to `true`. 