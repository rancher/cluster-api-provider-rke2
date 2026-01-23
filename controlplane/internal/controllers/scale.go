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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/storage/names"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/labels/format"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/controlplane/internal/util/ssa"
	rke2 "github.com/rancher/cluster-api-provider-rke2/pkg/rke2"
	rke2util "github.com/rancher/cluster-api-provider-rke2/pkg/util"
)

func (r *RKE2ControlPlaneReconciler) initializeControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
	controlPlane *rke2.ControlPlane,
) (ctrl.Result, error) {
	logger := controlPlane.Logger()

	// Perform an uncached read of all the owned machines. This check is in place to make sure
	// that the controller cache is not misbehaving and we end up initializing the cluster more than once.
	ownedMachines, err := r.managementClusterUncached.GetMachinesForCluster(ctx, cluster, collections.OwnedMachines(rcp))
	if err != nil {
		logger.Error(err, "failed to perform an uncached read of control plane machines for cluster")

		return ctrl.Result{}, err
	}

	if len(ownedMachines) > 0 {
		return ctrl.Result{}, errors.Errorf(
			"control plane has already been initialized, found %d owned machine for cluster %s/%s: controller cache or management cluster is misbehaving",
			len(ownedMachines), cluster.Namespace, cluster.Name,
		)
	}

	bootstrapSpec := controlPlane.InitialControlPlaneConfig()

	fd, err := controlPlane.NextFailureDomainForScaleUp(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to determine failure domain with the fewest number of up-to-date machines")
	}

	if err := r.cloneConfigsAndGenerateMachine(ctx, cluster, rcp, bootstrapSpec, fd); err != nil {
		logger.Error(err, "Failed to create initial control plane Machine")
		r.recorder.Eventf(
			rcp,
			corev1.EventTypeWarning,
			"FailedInitialization",
			"Failed to create initial control plane Machine for cluster %s/%s control plane: %v",
			cluster.Namespace,
			cluster.Name,
			err)

		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are additional operations to perform
	return ctrl.Result{Requeue: true}, nil
}

func (r *RKE2ControlPlaneReconciler) scaleUpControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
	controlPlane *rke2.ControlPlane,
) (ctrl.Result, error) {
	logger := controlPlane.Logger()

	// Run preflight checks to ensure that the control plane is stable before proceeding with a scale up/scale down operation; if not, wait.
	if result := r.preflightChecks(ctx, controlPlane); !result.IsZero() {
		return result, nil
	}

	// Create the bootstrap configuration
	bootstrapSpec := controlPlane.JoinControlPlaneConfig()

	fd, err := controlPlane.NextFailureDomainForScaleUp(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to determine failure domain with the fewest number of up-to-date machines")
	}

	if err := r.cloneConfigsAndGenerateMachine(ctx, cluster, rcp, bootstrapSpec, fd); err != nil {
		logger.Error(err, "Failed to create additional control plane Machine")
		r.recorder.Eventf(
			rcp,
			corev1.EventTypeWarning,
			"FailedScaleUp",
			"Failed to create additional control plane Machine for cluster %s/%s control plane: %v",
			cluster.Namespace,
			cluster.Name,
			err,
		)

		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are other operations to perform
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

func (r *RKE2ControlPlaneReconciler) scaleDownControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
	controlPlane *rke2.ControlPlane,
	outdatedMachines collections.Machines,
) (ctrl.Result, error) {
	logger := controlPlane.Logger()

	// Pick the Machine that we should scale down.
	machineToDelete, err := selectMachineForScaleDown(ctx, controlPlane, outdatedMachines)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to select machine for scale down")
	}

	// Run preflight checks ensuring the control plane is stable before proceeding with a scale up/scale down operation; if not, wait.
	// Given that we're scaling down, we can exclude the machineToDelete from the preflight checks.
	if result := r.preflightChecks(ctx, controlPlane, machineToDelete); !result.IsZero() {
		return result, nil
	}

	if machineToDelete == nil {
		logger.Info("Failed to pick control plane Machine to delete")

		return ctrl.Result{}, errors.New("failed to pick control plane Machine to delete")
	}

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting workload cluster: %w", err)
	}

	if controlPlane.UsesEmbeddedEtcd() {
		// If etcd leadership is on machine that is about to be deleted, move it to the newest member available.
		if _, found := controlPlane.RCP.Annotations[controlplanev1.LegacyRKE2ControlPlane]; !found {
			etcdLeaderCandidate := controlPlane.Machines.Newest()
			if err := workloadCluster.ForwardEtcdLeadership(ctx, machineToDelete, etcdLeaderCandidate); err != nil {
				logger.Error(err, "Failed to move leadership to candidate machine", "candidate", etcdLeaderCandidate.Name)

				return ctrl.Result{}, err
			}
		}
	}

	// NOTE: etcd member removal will be performed by the rke2-cleanup hook after machine completes drain & all volumes are detached.

	logger = logger.WithValues("machine", machineToDelete)
	if err := r.Delete(ctx, machineToDelete); err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to delete control plane machine")
		r.recorder.Eventf(rcp, corev1.EventTypeWarning, "FailedScaleDown",
			"Failed to delete control plane Machine %s for cluster %s/%s control plane: %v", machineToDelete.Name, cluster.Namespace, cluster.Name, err)

		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are additional operations to perform
	return ctrl.Result{Requeue: true}, nil
}

// preflightChecks checks if the control plane is stable before proceeding with a scale up/scale down operation,
// where stable means that:
// - There are no machine deletion in progress
// - All the health conditions on RCP are true.
// - All the health conditions on the control plane machines are true.
// If the control plane is not passing preflight checks, it requeue.
//
// NOTE: this func uses RCP conditions, it is required to call reconcileControlPlaneConditions before this.
func (r *RKE2ControlPlaneReconciler) preflightChecks(
	ctx context.Context,
	controlPlane *rke2.ControlPlane,
	excludeFor ...*clusterv1.Machine,
) ctrl.Result {
	logger := log.FromContext(ctx)

	// If there is no RCP-owned control-plane machines, then control-plane has not been initialized yet,
	// so it is considered ok to proceed.
	if controlPlane.Machines.Len() == 0 {
		return ctrl.Result{}
	}

	// If there are deleting machines, wait for the operation to complete.
	if controlPlane.HasDeletingMachine() {
		controlPlane.PreflightCheckResults.HasDeletingMachine = true
		logger.Info("Waiting for machines to be deleted", "Machines",
			strings.Join(controlPlane.Machines.Filter(collections.HasDeletionTimestamp).Names(),
				", ",
			))

		return ctrl.Result{RequeueAfter: deleteRequeueAfter}
	}

	// Check machine health conditions; if there are conditions with False or Unknown, then wait.
	allMachineHealthConditions := []string{
		controlplanev1.RKE2ControlPlaneMachineAgentHealthyCondition,
		controlplanev1.RKE2ControlPlaneMachineEtcdMemberHealthyCondition,
	}
	machineErrors := []error{}

loopmachines:
	for _, machine := range controlPlane.Machines {
		for _, excluded := range excludeFor {
			// If this machine should be excluded from the individual
			// health check, continue the out loop.
			if machine.Name == excluded.Name {
				continue loopmachines
			}
		}

		for _, condition := range allMachineHealthConditions {
			if err := preflightCheckCondition("machine", machine, condition); err != nil {
				machineErrors = append(machineErrors, err)
			}
		}
	}

	if len(machineErrors) > 0 {
		aggregatedError := kerrors.NewAggregate(machineErrors)
		r.recorder.Eventf(controlPlane.RCP, corev1.EventTypeWarning, "ControlPlaneUnhealthy",
			"Waiting for control plane to pass preflight checks to continue reconciliation: %v", aggregatedError)
		logger.Info("Waiting for control plane to pass preflight checks", "failures", aggregatedError.Error())

		return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}
	}

	return ctrl.Result{}
}

func preflightCheckCondition(kind string, obj *clusterv1.Machine, conditionType string) error {
	c := conditions.Get(obj, conditionType)
	if c == nil {
		return errors.Errorf("%s %s does not have %s condition", kind, obj.GetName(), conditionType)
	}

	if c.Status == metav1.ConditionFalse {
		return errors.Errorf("%s %s reports %s condition is false (%s)", kind, obj.GetName(), conditionType, c.Message)
	}

	if c.Status == metav1.ConditionUnknown {
		return errors.Errorf("%s %s reports %s condition is unknown (%s)", kind, obj.GetName(), conditionType, c.Message)
	}

	return nil
}

func selectMachineForScaleDown(
	ctx context.Context,
	controlPlane *rke2.ControlPlane,
	outdatedMachines collections.Machines,
) (*clusterv1.Machine, error) {
	machines := controlPlane.Machines

	switch {
	case controlPlane.MachineWithDeleteAnnotation(outdatedMachines).Len() > 0:
		machines = controlPlane.MachineWithDeleteAnnotation(outdatedMachines)
		controlPlane.Logger().V(5).Info("Inside the withDeleteAnnotation-outdated case", "machines", machines.Names())
	case controlPlane.MachineWithDeleteAnnotation(machines).Len() > 0:
		machines = controlPlane.MachineWithDeleteAnnotation(machines)
		controlPlane.Logger().V(5).Info("Inside the withDeleteAnnotation case", "machines", machines.Names())
	case outdatedMachines.Len() > 0:
		machines = outdatedMachines
		controlPlane.Logger().V(5).Info("Inside the Outdated case", "machines", machines.Names())
	case machines.Filter(collections.Not(collections.IsReady())).Len() > 0:
		machines = machines.Filter(collections.Not(collections.IsReady()))
		controlPlane.Logger().V(5).Info("Inside the IsReady case", "machines", machines.Names())
	}

	return controlPlane.MachineInFailureDomainWithMostMachines(ctx, machines)
}

func (r *RKE2ControlPlaneReconciler) cloneConfigsAndGenerateMachine(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
	bootstrapSpec *bootstrapv1.RKE2ConfigSpec,
	failureDomain string,
) error {
	var errs []error

	// Since the cloned resource should eventually have a controller ref for the Machine, we create an
	// OwnerReference here without the Controller field set
	infraCloneOwner := &metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "RKE2ControlPlane",
		Name:       rcp.Name,
		UID:        rcp.UID,
	}

	apiVersion, err := rke2util.GetAPIVersion(ctx, r.Client, rcp.Spec.MachineTemplate.Spec.InfrastructureRef.GroupKind())
	if err != nil {
		return errors.Wrap(err, "failed to get api version for kind "+rcp.Spec.MachineTemplate.Spec.InfrastructureRef.Kind)
	}

	// Clone the infrastructure template
	infraMachine, infraRef, err := external.CreateFromTemplate(ctx, &external.CreateFromTemplateInput{
		Client: r.Client,
		TemplateRef: &corev1.ObjectReference{
			APIVersion: apiVersion,
			Kind:       rcp.Spec.MachineTemplate.Spec.InfrastructureRef.Kind,
			Name:       rcp.Spec.MachineTemplate.Spec.InfrastructureRef.Name,
			Namespace:  rcp.Namespace,
		},
		Namespace:   rcp.Namespace,
		OwnerRef:    infraCloneOwner,
		ClusterName: cluster.Name,
		Labels:      rke2.ControlPlaneLabelsForCluster(cluster.Name),
	})
	if err != nil {
		// Safe to return early here since no resources have been created yet.
		return errors.Wrap(err, "failed to clone infrastructure template")
	}

	// Clone the bootstrap configuration
	bootstrapConfig, bootstrapRef, err := r.generateRKE2Config(ctx, rcp, cluster, bootstrapSpec)
	if err != nil {
		errs = append(errs, errors.Wrap(err, "failed to generate bootstrap config"))
	}

	// Only proceed to generating the Machine if we haven't encountered an error
	if len(errs) == 0 {
		if err := r.createMachine(ctx, rcp, cluster, infraRef, bootstrapRef, failureDomain); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to create Machine"))
		}
	}

	// If we encountered any errors, attempt to clean up any dangling resources
	if len(errs) > 0 {
		if err := r.cleanupFromGeneration(ctx, infraMachine, bootstrapConfig); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to cleanup generated resources"))
		}

		return kerrors.NewAggregate(errs)
	}

	return nil
}

func (r *RKE2ControlPlaneReconciler) cleanupFromGeneration(ctx context.Context, objects ...client.Object) error {
	var errs []error

	for _, obj := range objects {
		if obj == nil {
			continue
		}

		if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, errors.Wrap(err, "failed to cleanup generated resources after error"))
		}
	}

	return kerrors.NewAggregate(errs)
}

func (r *RKE2ControlPlaneReconciler) generateRKE2Config(
	ctx context.Context,
	rcp *controlplanev1.RKE2ControlPlane,
	cluster *clusterv1.Cluster,
	spec *bootstrapv1.RKE2ConfigSpec,
) (*bootstrapv1.RKE2Config, clusterv1.ContractVersionedObjectReference, error) {
	// Create an owner reference without a controller reference because the owning controller is the machine controller
	owner := metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "RKE2ControlPlane",
		Name:       rcp.Name,
		UID:        rcp.UID,
	}

	bootstrapConfig := &bootstrapv1.RKE2Config{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.SimpleNameGenerator.GenerateName(rcp.Name + "-"),
			Namespace:       rcp.Namespace,
			Labels:          rke2.ControlPlaneLabelsForCluster(cluster.Name),
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: *spec,
	}

	if err := r.Create(ctx, bootstrapConfig); err != nil {
		return nil, clusterv1.ContractVersionedObjectReference{}, errors.Wrap(err, "Failed to create bootstrap configuration")
	}

	bootstrapRef := clusterv1.ContractVersionedObjectReference{
		APIGroup: bootstrapv1.GroupVersion.Group,
		Kind:     "RKE2Config",
		Name:     bootstrapConfig.GetName(),
	}

	return bootstrapConfig, bootstrapRef, nil
}

// UpdateExternalObject updates the external object with the labels and annotations from RKE2ControlPlane.
func (r *RKE2ControlPlaneReconciler) UpdateExternalObject(
	ctx context.Context,
	obj client.Object,
	rcp *controlplanev1.RKE2ControlPlane,
	cluster *clusterv1.Cluster,
) error {
	updatedObject := &unstructured.Unstructured{}
	updatedObject.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	updatedObject.SetNamespace(obj.GetNamespace())
	updatedObject.SetName(obj.GetName())
	// Set the UID to ensure that Server-Side-Apply only performs an update
	// and does not perform an accidental create.
	updatedObject.SetUID(obj.GetUID())

	// Update labels
	updatedObject.SetLabels(ControlPlaneMachineLabelsForCluster(rcp, cluster.Name))
	// Update annotations
	updatedObject.SetAnnotations(rcp.Spec.MachineTemplate.ObjectMeta.Annotations)

	if err := ssa.Patch(ctx, r.Client, rke2ManagerName, updatedObject, ssa.WithCachingProxy{Cache: r.ssaCache, Original: obj}); err != nil {
		return errors.Wrapf(err, "failed to update %s", obj.GetObjectKind().GroupVersionKind().Kind)
	}

	return nil
}

// createMachine creates a new Machine object for the control plane.
func (r *RKE2ControlPlaneReconciler) createMachine(
	ctx context.Context,
	rcp *controlplanev1.RKE2ControlPlane,
	cluster *clusterv1.Cluster,
	infraRef, bootstrapRef clusterv1.ContractVersionedObjectReference,
	failureDomain string,
) error {
	machine, err := r.computeDesiredMachine(rcp, cluster, infraRef, bootstrapRef, failureDomain, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create Machine: failed to compute desired Machine")
	}

	patchOptions := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(rke2ManagerName),
	}

	if err := r.Patch(ctx, machine, client.Apply, patchOptions...); err != nil {
		return errors.Wrap(err, "failed to create Machine: apply failed")
	}

	return nil
}

// UpdateMachine updates an existing Machine object for the control plane.
func (r *RKE2ControlPlaneReconciler) UpdateMachine(
	ctx context.Context,
	machine *clusterv1.Machine,
	rcp *controlplanev1.RKE2ControlPlane,
	cluster *clusterv1.Cluster,
) (*clusterv1.Machine, error) {
	updatedMachine, err := r.computeDesiredMachine(
		rcp, cluster,
		machine.Spec.InfrastructureRef, machine.Spec.Bootstrap.ConfigRef,
		machine.Spec.FailureDomain, machine,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to update Machine: failed to compute desired Machine")
	}

	patchOptions := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(rke2ManagerName),
	}
	if err := r.Patch(ctx, updatedMachine, client.Apply, patchOptions...); err != nil {
		return nil, errors.Wrap(err, "failed to update Machine: apply failed")
	}

	return updatedMachine, nil
}

// computeDesiredMachine computes the desired Machine.
// This Machine will be used during reconciliation to:
// * create a new Machine
// * update an existing Machine
// Because we are using Server-Side-Apply we always have to calculate the full object.
// There are small differences in how we calculate the Machine depending on if it
// is a create or update. Example: for a new Machine we have to calculate a new name,
// while for an existing Machine we have to use the name of the existing Machine.
func (r *RKE2ControlPlaneReconciler) computeDesiredMachine(
	rcp *controlplanev1.RKE2ControlPlane,
	cluster *clusterv1.Cluster, infraRef,
	bootstrapRef clusterv1.ContractVersionedObjectReference,
	failureDomain string,
	existingMachine *clusterv1.Machine,
) (*clusterv1.Machine, error) {
	var (
		machineName string
		machineUID  types.UID
		version     string
	)

	annotations := map[string]string{}

	if existingMachine == nil {
		// Creating a new machine
		machineName = names.SimpleNameGenerator.GenerateName(rcp.Name + "-")

		desiredVersion := rcp.GetDesiredVersion()
		version = desiredVersion

		// Machine's bootstrap config may be missing RKE2Config if it is not the first machine in the control plane.
		// We store RKE2Config as annotation here to detect any changes in RCP RKE2Config and rollout the machine if any.
		serverConfig, err := json.Marshal(rcp.Spec.ServerConfig)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal cluster configuration")
		}

		annotations[controlplanev1.RKE2ServerConfigurationAnnotation] = string(serverConfig)
		annotations[controlplanev1.PreTerminateHookCleanupAnnotation] = ""
	} else {
		// Updating an existing machine
		machineName = existingMachine.Name
		machineUID = existingMachine.UID
		version = existingMachine.Spec.Version

		// For existing machine only set the ClusterConfiguration annotation if the machine already has it.
		// We should not add the annotation if it was missing in the first place because we do not have enough
		// information.
		if serverConfig, ok := existingMachine.Annotations[controlplanev1.RKE2ServerConfigurationAnnotation]; ok {
			annotations[controlplanev1.RKE2ServerConfigurationAnnotation] = serverConfig
		}
	}

	// Construct the basic Machine.
	desiredMachine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			UID:       machineUID,
			Name:      machineName,
			Namespace: rcp.Namespace,
			// Note: by setting the ownerRef on creation we signal to the Machine controller that this is not a stand-alone Machine.
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(rcp, controlplanev1.GroupVersion.WithKind(rke2ControlPlaneKind)),
			},
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       cluster.Name,
			Version:           version,
			FailureDomain:     failureDomain,
			InfrastructureRef: infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
		},
	}

	// Set the in-place mutable fields.
	// When we create a new Machine we will just create the Machine with those fields.
	// When we update an existing Machine will we update the fields on the existing Machine (in-place mutate).

	// Set labels
	desiredMachine.Labels = ControlPlaneMachineLabelsForCluster(rcp, cluster.Name)

	// Set annotations
	// Add the annotations from the MachineTemplate.
	// Note: we intentionally don't use the map directly to ensure we don't modify the map in RKE2ControlPlane.
	for k, v := range rcp.Spec.MachineTemplate.ObjectMeta.Annotations {
		desiredMachine.Annotations[k] = v
	}

	for k, v := range annotations {
		desiredMachine.Annotations[k] = v
	}

	// Set other in-place mutable fields
	desiredMachine.Spec.Deletion = clusterv1.MachineDeletionSpec{
		NodeDrainTimeoutSeconds:        rcp.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds,
		NodeDeletionTimeoutSeconds:     rcp.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds,
		NodeVolumeDetachTimeoutSeconds: rcp.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds,
	}

	return desiredMachine, nil
}

// ControlPlaneMachineLabelsForCluster returns a set of labels to add to a control plane machine for this specific cluster.
func ControlPlaneMachineLabelsForCluster(rcp *controlplanev1.RKE2ControlPlane, clusterName string) map[string]string {
	labels := map[string]string{}

	// Add the labels from the MachineTemplate.
	// Note: we intentionally don't use the map directly to ensure we don't modify the map in RKE2ControlPlane.
	for k, v := range rcp.Spec.MachineTemplate.ObjectMeta.Labels {
		labels[k] = v
	}

	// Always force these labels over the ones coming from the spec.
	labels[clusterv1.ClusterNameLabel] = clusterName
	labels[clusterv1.MachineControlPlaneLabel] = ""
	// Note: MustFormatValue is used here as the label value can be a hash if the control plane name is longer than 63 characters.
	labels[clusterv1.MachineControlPlaneNameLabel] = format.MustFormatValue(rcp.Name)

	return labels
}
