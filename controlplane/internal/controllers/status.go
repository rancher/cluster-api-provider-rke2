/*
Copyright 2022 SUSE.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/registration"
	"github.com/rancher/cluster-api-provider-rke2/pkg/rke2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/secret"
	rke2util "github.com/rancher/cluster-api-provider-rke2/pkg/util"
)

// updateStatus is called after every reconciliation loop in a defer statement to always make sure we have the
// RKE2ControlPlane status up-to-date.
// nolint:gocyclo
func (r *RKE2ControlPlaneReconciler) updateStatus(ctx context.Context, rcp *controlplanev1.RKE2ControlPlane, cluster *clusterv1.Cluster) error {
	logger := log.FromContext(ctx)

	if cluster == nil {
		logger.Info("Cluster is nil, skipping status update")

		return nil
	}

	if rcp.Spec.Replicas == nil {
		logger.Info("RKE2ControlPlane.Spec.Replicas is nil, skipping status update")

		return nil
	}

	ownedMachines, err := r.managementCluster.GetMachinesForCluster(
		ctx,
		cluster,
		collections.OwnedMachines(rcp))
	if err != nil {
		return errors.Wrap(err, "failed to get list of owned machines")
	}

	if ownedMachines == nil {
		logger.Info("Owned machines list is nil, skipping status update")

		return nil
	}

	readyMachines := ownedMachines.Filter(collections.IsReady())
	if readyMachines == nil {
		logger.Info("Ready machines list is nil, skipping status update")

		return nil
	}

	for _, readyMachine := range readyMachines {
		logger.V(3).Info("Ready Machine : " + readyMachine.Name)
	}

	controlPlane, err := rke2.NewControlPlane(ctx, r.managementCluster, r.Client, cluster, rcp, ownedMachines)
	if err != nil {
		logger.Error(err, "failed to initialize control plane")

		return err
	}

	setReplicas(ctx, rcp, ownedMachines)
	setInitializedCondition(ctx, controlPlane.RCP)
	setMachinesReadyCondition(ctx, controlPlane.RCP, controlPlane.Machines)

	kubeconfigSecret := corev1.Secret{}

	err = r.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      secret.Name(cluster.Name, secret.Kubeconfig),
	}, &kubeconfigSecret)
	if err != nil {
		logger.Info("Kubeconfig secret does not yet exist")

		return err
	}

	kubeConfig := kubeconfigSecret.Data[secret.KubeconfigDataName]
	if kubeConfig == nil {
		return errors.New("unable to find a value entry in the kubeconfig secret")
	}

	rcp.Status.ReadyReplicas = ptr.To(rke2util.SafeInt32(len(readyMachines)))

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		logger.Error(err, "Failed to get remote client for workload cluster", "cluster key", util.ObjectKey(cluster))

		return fmt.Errorf("getting workload cluster: %w", err)
	}

	if workloadCluster == nil {
		logger.Info("Workload cluster is nil, skipping status update")

		return nil
	}

	status := workloadCluster.ClusterStatus(ctx)

	if status.HasRKE2ServingSecret {
		rcp.Status.Initialization.ControlPlaneInitialized = ptr.To(true)
	}

	if len(ownedMachines) == 0 || len(readyMachines) == 0 {
		logger.Info(fmt.Sprintf("No Control Plane Machines exist or are ready for RKE2ControlPlane %s/%s", rcp.Namespace, rcp.Name))

		return nil
	}

	availableCPMachines := readyMachines

	registrationMethod, err := registration.NewRegistrationMethod(string(rcp.Spec.RegistrationMethod))
	if err != nil {
		logger.Error(err, "Failed to get node registration method")

		return fmt.Errorf("getting node registration method: %w", err)
	}

	validIPAddresses, err := registrationMethod(cluster, rcp, availableCPMachines)
	if err != nil {
		logger.Error(err, "Failed to get registration addresses")

		return fmt.Errorf("getting registration addresses: %w", err)
	}

	rcp.Status.AvailableServerIPs = validIPAddresses
	if len(rcp.Status.AvailableServerIPs) == 0 {
		return errors.New("some Control Plane machines exist and are ready but they have no IP Address available")
	}

	if len(readyMachines) == len(ownedMachines) {
		rcp.Status.Initialization.ControlPlaneInitialized = ptr.To(true)
	}

	if ptr.Deref(rcp.Status.Initialization.ControlPlaneInitialized, false) {
		v1beta1conditions.MarkTrue(rcp, controlplanev1.AvailableV1Beta1Condition)
		conditions.Set(rcp, metav1.Condition{
			Type:   controlplanev1.RKE2ControlPlaneAvailableCondition,
			Status: metav1.ConditionTrue,
			Reason: controlplanev1.RKE2ControlPlaneAvailableReason,
		})
	}

	lowestVersion := controlPlane.Machines.LowestVersion()
	if lowestVersion != "" {
		controlPlane.RCP.Status.Version = lowestVersion
	}

	// Surface lastRemediation data in status.
	// LastRemediation is the remediation currently in progress, in any, or the
	// most recent of the remediation we are keeping track on machines.
	var lastRemediation *RemediationData

	if v, ok := controlPlane.RCP.Annotations[controlplanev1.RemediationInProgressAnnotation]; ok {
		remediationData, err := RemediationDataFromAnnotation(v)
		if err != nil {
			return err
		}

		lastRemediation = remediationData
	} else {
		for _, m := range controlPlane.Machines.UnsortedList() {
			if v, ok := m.Annotations[controlplanev1.RemediationForAnnotation]; ok {
				remediationData, err := RemediationDataFromAnnotation(v)
				if err != nil {
					return err
				}

				if lastRemediation == nil || lastRemediation.Timestamp.Time.Before(remediationData.Timestamp.Time) {
					lastRemediation = remediationData
				}
			}
		}
	}

	if lastRemediation != nil {
		controlPlane.RCP.Status.LastRemediation = lastRemediation.ToStatus()
	}

	logger.Info("Successfully updated RKE2ControlPlane Status", "namespace", rcp.Namespace, "name", rcp.Name)

	return nil
}

// updateV1Beta1Status is called after every reconciliation loop in a defer statement to always make sure we have the
// RKE2ControlPlane status up-to-date.
// nolint:gocyclo
func (r *RKE2ControlPlaneReconciler) updateV1Beta1Status(ctx context.Context, rcp *controlplanev1.RKE2ControlPlane, cluster *clusterv1.Cluster) error { // nolint:lll
	logger := log.FromContext(ctx)

	if cluster == nil {
		logger.Info("Cluster is nil, skipping status update")

		return nil
	}

	if rcp.Spec.Replicas == nil {
		logger.Info("RKE2ControlPlane.Spec.Replicas is nil, skipping status update")

		return nil
	}

	ownedMachines, err := r.managementCluster.GetMachinesForCluster(
		ctx,
		cluster,
		collections.OwnedMachines(rcp))
	if err != nil {
		return errors.Wrap(err, "failed to get list of owned machines")
	}

	if ownedMachines == nil {
		logger.Info("Owned machines list is nil, skipping status update")

		return nil
	}

	readyMachines := ownedMachines.Filter(collections.IsReady())
	if readyMachines == nil {
		logger.Info("Ready machines list is nil, skipping status update")

		return nil
	}

	for _, readyMachine := range readyMachines {
		logger.V(3).Info("Ready Machine : " + readyMachine.Name)
	}

	controlPlane, err := rke2.NewControlPlane(ctx, r.managementCluster, r.Client, cluster, rcp, ownedMachines)
	if err != nil {
		logger.Error(err, "failed to initialize control plane")

		return err
	}

	if rcp.Status.Deprecated == nil {
		rcp.Status.Deprecated = &controlplanev1.RKE2ControlPlaneDeprecatedStatus{}
	}

	if rcp.Status.Deprecated.V1Beta1 == nil {
		rcp.Status.Deprecated.V1Beta1 = &controlplanev1.RKE2ControlPlaneV1Beta1DeprecatedStatus{}
	}

	rcp.Status.Deprecated.V1Beta1.UpdatedReplicas = rke2util.SafeInt32(len(controlPlane.UpToDateMachines(ctx))) // nolint:staticcheck

	replicas := rke2util.SafeInt32(len(ownedMachines))
	desiredReplicas := *rcp.Spec.Replicas

	// set basic data that does not require interacting with the workload cluster
	// ReadyReplicas and UnavailableReplicas are set in case the function returns before updating them
	rcp.Status.Deprecated.V1Beta1.ReadyReplicas = 0              // nolint:staticcheck
	rcp.Status.Deprecated.V1Beta1.UnavailableReplicas = replicas // nolint:staticcheck

	// Return early if the deletion timestamp is set, because we don't want to try to connect to the workload cluster
	// and we don't want to report resize condition (because it is set to deletin into reconcile delete).
	if !rcp.DeletionTimestamp.IsZero() {
		return nil
	}

	switch {
	// We are scaling up
	case replicas < desiredReplicas:
		v1beta1conditions.MarkFalse(
			rcp,
			controlplanev1.ResizedV1Beta1Condition,
			controlplanev1.ScalingUpV1Beta1Reason,
			clusterv1.ConditionSeverityWarning,
			"Scaling up control plane to %d replicas (actual %d)",
			desiredReplicas,
			replicas)

	// We are scaling down
	case replicas > desiredReplicas:
		v1beta1conditions.MarkFalse(
			rcp,
			controlplanev1.ResizedV1Beta1Condition,
			controlplanev1.ScalingDownV1Beta1Reason,
			clusterv1.ConditionSeverityWarning,
			"Scaling down control plane to %d replicas (actual %d)",
			desiredReplicas,
			replicas)

	default:
		// make sure last resize operation is marked as completed.
		// NOTE: we are checking the number of machines ready so we report resize completed only when the machines
		// are actually provisioned (vs reporting completed immediately after the last machine object is created).
		if rke2util.SafeInt32(len(readyMachines)) == replicas {
			v1beta1conditions.MarkTrue(rcp, controlplanev1.ResizedV1Beta1Condition)
		}
	}

	kubeconfigSecret := corev1.Secret{}

	err = r.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      secret.Name(cluster.Name, secret.Kubeconfig),
	}, &kubeconfigSecret)
	if err != nil {
		logger.Info("Kubeconfig secret does not yet exist")

		return err
	}

	kubeConfig := kubeconfigSecret.Data[secret.KubeconfigDataName]
	if kubeConfig == nil {
		return errors.New("unable to find a value entry in the kubeconfig secret")
	}

	rcp.Status.Deprecated.V1Beta1.ReadyReplicas = rke2util.SafeInt32(len(readyMachines))                  // nolint:staticcheck
	rcp.Status.Deprecated.V1Beta1.UnavailableReplicas = replicas - rke2util.SafeInt32(len(readyMachines)) // nolint:staticcheck

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		logger.Error(err, "Failed to get remote client for workload cluster", "cluster key", util.ObjectKey(cluster))

		return fmt.Errorf("getting workload cluster: %w", err)
	}

	if workloadCluster == nil {
		logger.Info("Workload cluster is nil, skipping status update")

		return nil
	}

	status := workloadCluster.ClusterStatus(ctx)

	if status.HasRKE2ServingSecret {
		rcp.Status.Initialization.ControlPlaneInitialized = ptr.To(true)
	}

	if len(ownedMachines) == 0 || len(readyMachines) == 0 {
		logger.Info(fmt.Sprintf("No Control Plane Machines exist or are ready for RKE2ControlPlane %s/%s", rcp.Namespace, rcp.Name))

		return nil
	}

	availableCPMachines := readyMachines

	registrationMethod, err := registration.NewRegistrationMethod(string(rcp.Spec.RegistrationMethod))
	if err != nil {
		logger.Error(err, "Failed to get node registration method")

		return fmt.Errorf("getting node registration method: %w", err)
	}

	validIPAddresses, err := registrationMethod(cluster, rcp, availableCPMachines)
	if err != nil {
		logger.Error(err, "Failed to get registration addresses")

		return fmt.Errorf("getting registration addresses: %w", err)
	}

	rcp.Status.AvailableServerIPs = validIPAddresses
	if len(rcp.Status.AvailableServerIPs) == 0 {
		return errors.New("some Control Plane machines exist and are ready but they have no IP Address available")
	}

	if len(readyMachines) == len(ownedMachines) {
		rcp.Status.Initialization.ControlPlaneInitialized = ptr.To(true)
	}

	if ptr.Deref(rcp.Status.Initialization.ControlPlaneInitialized, false) {
		v1beta1conditions.MarkTrue(rcp, controlplanev1.AvailableV1Beta1Condition)
		conditions.Set(rcp, metav1.Condition{
			Type:   controlplanev1.RKE2ControlPlaneAvailableCondition,
			Status: metav1.ConditionTrue,
			Reason: controlplanev1.RKE2ControlPlaneAvailableReason,
		})
	}

	lowestVersion := controlPlane.Machines.LowestVersion()
	if lowestVersion != "" {
		controlPlane.RCP.Status.Version = lowestVersion
	}

	// Surface lastRemediation data in status.
	// LastRemediation is the remediation currently in progress, in any, or the
	// most recent of the remediation we are keeping track on machines.
	var lastRemediation *RemediationData

	if v, ok := controlPlane.RCP.Annotations[controlplanev1.RemediationInProgressAnnotation]; ok {
		remediationData, err := RemediationDataFromAnnotation(v)
		if err != nil {
			return err
		}

		lastRemediation = remediationData
	} else {
		for _, m := range controlPlane.Machines.UnsortedList() {
			if v, ok := m.Annotations[controlplanev1.RemediationForAnnotation]; ok {
				remediationData, err := RemediationDataFromAnnotation(v)
				if err != nil {
					return err
				}

				if lastRemediation == nil || lastRemediation.Timestamp.Time.Before(remediationData.Timestamp.Time) {
					lastRemediation = remediationData
				}
			}
		}
	}

	if lastRemediation != nil {
		controlPlane.RCP.Status.LastRemediation = lastRemediation.ToStatus()
	}

	logger.Info("Successfully updated RKE2ControlPlane V1Beta1 Status", "namespace", rcp.Namespace, "name", rcp.Name)

	return nil
}

func setReplicas(_ context.Context, rcp *controlplanev1.RKE2ControlPlane, machines collections.Machines) {
	var readyReplicas, availableReplicas, upToDateReplicas int32

	for _, machine := range machines {
		if conditions.IsTrue(machine, clusterv1.MachineReadyCondition) {
			readyReplicas++
		}

		if conditions.IsTrue(machine, clusterv1.MachineAvailableCondition) {
			availableReplicas++
		}

		if conditions.IsTrue(machine, clusterv1.MachineUpToDateCondition) {
			upToDateReplicas++
		}
	}

	rcp.Status.Replicas = ptr.To(rke2util.SafeInt32(len(machines)))
	rcp.Status.ReadyReplicas = ptr.To(readyReplicas)
	rcp.Status.AvailableReplicas = ptr.To(availableReplicas)
	rcp.Status.UpToDateReplicas = ptr.To(upToDateReplicas)
}

func setInitializedCondition(_ context.Context, rcp *controlplanev1.RKE2ControlPlane) {
	if ptr.Deref(rcp.Status.Initialization.ControlPlaneInitialized, false) {
		conditions.Set(rcp, metav1.Condition{
			Type:   controlplanev1.RKE2ControlPlaneInitializedCondition,
			Status: metav1.ConditionTrue,
			Reason: controlplanev1.RKE2ControlPlaneInitializedReason,
		})

		return
	}

	conditions.Set(rcp, metav1.Condition{
		Type:   controlplanev1.RKE2ControlPlaneInitializedCondition,
		Status: metav1.ConditionFalse,
		Reason: controlplanev1.RKE2ControlPlaneNotInitializedReason,
	})
}

func setMachinesReadyCondition(ctx context.Context, rcp *controlplanev1.RKE2ControlPlane, machines collections.Machines) {
	if len(machines) == 0 {
		conditions.Set(rcp, metav1.Condition{
			Type:   controlplanev1.RKE2ControlPlaneMachinesReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: controlplanev1.RKE2ControlPlaneMachinesReadyNoReplicasReason,
		})
		return
	}

	readyCondition, err := conditions.NewAggregateCondition(
		machines.UnsortedList(), clusterv1.MachineReadyCondition,
		conditions.TargetConditionType(controlplanev1.RKE2ControlPlaneMachinesReadyCondition),
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					controlplanev1.RKE2ControlPlaneMachinesNotReadyReason,
					controlplanev1.RKE2ControlPlaneMachinesReadyUnknownReason,
					controlplanev1.RKE2ControlPlaneMachinesReadyReason,
				)),
			),
		},
	)
	if err != nil {
		conditions.Set(rcp, metav1.Condition{
			Type:    controlplanev1.RKE2ControlPlaneMachinesReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.RKE2ControlPlaneMachinesReadyInternalErrorReason,
			Message: "Please check controller logs for errors",
		})

		log := ctrl.LoggerFrom(ctx)
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", clusterv1.MachineReadyCondition))
		return
	}

	conditions.Set(rcp, *readyCondition)
}
