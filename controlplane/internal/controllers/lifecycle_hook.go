/*
Copyright 2025 SUSE.

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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klog "k8s.io/klog/v2" //nolint: gci
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime" //nolint: gci
	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/rke2"
)

const (
	trueString = "true"
)

// reconcileLifecycleHooks triggers the reconcile of all implemented lifecycle hooks.
// **NOTE** keep the hooks in the expected lifecycle order.
func (r *RKE2ControlPlaneReconciler) reconcileLifecycleHooks(ctx context.Context, controlPlane *rke2.ControlPlane) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	log.V(5).Info("Reconciling pre-drain hooks")

	preDrainResult, err := r.reconcilePreDrainHook(ctx, controlPlane)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.V(5).Info("Reconciling pre-terminate hooks")

	preTerminateResult, err := r.reconcilePreTerminateHook(ctx, controlPlane)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !preDrainResult.IsZero() {
		return preDrainResult, nil
	}

	if !preTerminateResult.IsZero() {
		return preTerminateResult, nil
	}

	return ctrl.Result{}, nil
}

func (r *RKE2ControlPlaneReconciler) reconcilePreDrainHook(ctx context.Context, controlPlane *rke2.ControlPlane) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	cleanup := false

	if controlPlane.RCP.Annotations == nil {
		log.V(5).Info("RKE2ControlPlane has no annotations. Cleaning up pre-drain hooks if needed.")

		cleanup = true
	}

	value, found := controlPlane.RCP.Annotations[controlplanev1.LoadBalancerExclusionAnnotation]

	if !found {
		log.V(5).Info("RKE2ControlPlane has no load balancer exclusion annotation. Cleaning up pre-drain hooks if needed.")

		cleanup = true
	}

	if value != trueString {
		log.V(5).Info("RKE2ControlPlane load balancer exclusion annotation is not set to 'true'. Cleaning up pre-drain hooks if needed.")

		cleanup = true
	}

	if cleanup {
		if err := cleanupHookOnAllMachines(ctx, controlPlane, controlplanev1.PreDrainLoadbalancerExclusionAnnotation); err != nil {
			return ctrl.Result{}, fmt.Errorf("cleaning up hook annotation %s on machines: %w", controlplanev1.PreDrainLoadbalancerExclusionAnnotation, err)
		}

		log.V(5).Info("load-balancer-exclusion annotation is not active. Nothing to do.")

		return ctrl.Result{}, nil
	}

	if err := applyHookToActiveMachines(ctx, controlPlane, controlplanev1.PreDrainLoadbalancerExclusionAnnotation); err != nil {
		return ctrl.Result{}, fmt.Errorf("applying hook annotation %s to machines: %w", controlplanev1.PreDrainLoadbalancerExclusionAnnotation, err)
	}

	if !controlPlane.HasDeletingMachine() {
		log.V(5).Info("No control plane machines are deleting. No hook to reconcile.")

		return ctrl.Result{}, nil
	}

	deletingMachine := getDeletingMachineWithHook(ctx,
		controlPlane,
		controlplanev1.PreDrainLoadbalancerExclusionAnnotation)
	if deletingMachine == nil {
		log.V(5).Info("Waiting on other machines to be deleted.")

		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	log = log.WithValues("Machine", klog.KObj(deletingMachine))
	ctx = ctrl.LoggerInto(ctx, log)

	if controlPlane.Machines.Len() < 2 {
		log.Info("Only one control plane machine left. Skipping exclusion from load balancer")

		if err := r.removeHookAnnotationFromMachine(ctx, deletingMachine, controlplanev1.PreDrainLoadbalancerExclusionAnnotation); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Apply the exclude-from-external-load-balancers label on the Node
	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting workload cluster: %w", err)
	}

	if err := workloadCluster.ApplyLabelOnNode(ctx, deletingMachine, corev1.LabelNodeExcludeBalancers, trueString); err != nil {
		return ctrl.Result{}, fmt.Errorf("applying label %s on machine %s node: %w", corev1.LabelNodeExcludeBalancers, deletingMachine.Name, err)
	}

	if err := r.removeHookAnnotationFromMachine(ctx, deletingMachine, controlplanev1.PreDrainLoadbalancerExclusionAnnotation); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Waiting for Machines to be deleted", "machines",
		strings.Join(controlPlane.Machines.Filter(collections.HasDeletionTimestamp).Names(), ", "))

	return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
}

func (r *RKE2ControlPlaneReconciler) reconcilePreTerminateHook(ctx context.Context, controlPlane *rke2.ControlPlane) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if err := applyHookToActiveMachines(ctx, controlPlane, controlplanev1.PreTerminateHookCleanupAnnotation); err != nil {
		return ctrl.Result{}, fmt.Errorf("applying hook annotation %s to machines: %w", controlplanev1.PreTerminateHookCleanupAnnotation, err)
	}

	if !controlPlane.HasDeletingMachine() {
		log.V(5).Info("No control plane machines are deleting. No hook to reconcile.")

		return ctrl.Result{}, nil
	}

	deletingMachine := getDeletingMachineWithHook(ctx,
		controlPlane,
		controlplanev1.PreTerminateHookCleanupAnnotation)
	if deletingMachine == nil {
		log.V(5).Info("Waiting on other machines to be deleted.")

		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// Return early if there are other pre-terminate hooks for the Machine.
	// The CAPRKE2 pre-terminate hook should be the one executed last, so that kubelet
	// is still working while other pre-terminate hooks are run.
	if machineHasOtherHooks(deletingMachine, clusterv1.PreTerminateDeleteHookAnnotationPrefix, controlplanev1.PreTerminateHookCleanupAnnotation) {
		log.V(5).Info("Waiting on other hooks to be handled.")

		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	log = log.WithValues("Machine", klog.KObj(deletingMachine))
	ctx = ctrl.LoggerInto(ctx, log)

	// The following will execute and remove the pre-terminate hook from the Machine.

	// Skip leader change for legacy CP
	_, found := controlPlane.RCP.Annotations[controlplanev1.LegacyRKE2ControlPlane]

	// If we have more than 1 Machine and etcd is managed we forward etcd leadership and remove the member
	// to keep the etcd cluster healthy.
	if controlPlane.Machines.Len() > 1 && !found {
		workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
		if err != nil {
			return ctrl.Result{},
				fmt.Errorf("removing etcd member for deleting Machine %s: failed to create client to workload cluster", klog.KObj(deletingMachine))
		}

		if controlPlane.UsesEmbeddedEtcd() {
			// Note: In regular deletion cases (remediation, scale down) the leader should have been already moved.
			// We're doing this again here in case the Machine became leader again or the Machine deletion was
			// triggered in another way (e.g. a user running kubectl delete machine)
			etcdLeaderCandidate := controlPlane.Machines.Filter(collections.Not(collections.HasDeletionTimestamp)).Newest()
			if etcdLeaderCandidate != nil {
				if err := workloadCluster.ForwardEtcdLeadership(ctx, deletingMachine, etcdLeaderCandidate); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to move leadership to candidate Machine %s: %w", etcdLeaderCandidate.Name, err)
				}
			} else {
				log.Info("Skip forwarding etcd leadership, because there is no other control plane Machine without a deletionTimestamp")
			}

			// Note: Removing the etcd member will lead to the etcd and the kube-apiserver Pod on the Machine shutting down.
			if err := workloadCluster.RemoveEtcdMemberForMachine(ctx, deletingMachine); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove etcd member for deleting Machine %s: %w", klog.KObj(deletingMachine), err)
			}

			safelyRemoved, err := workloadCluster.IsEtcdMemberSafelyRemovedForMachine(ctx, deletingMachine)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("determining if etcd member is safely removed for Machine %s: %w", klog.KObj(deletingMachine), err)
			}

			if !safelyRemoved {
				log.Info("Waiting for etcd member for Machine to be safely removed", "machine", klog.KObj(deletingMachine))

				return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
			}
		}
	}

	if err := r.removePreTerminateHookAnnotationFromMachine(ctx, deletingMachine); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Waiting for Machines to be deleted", "machines",
		strings.Join(controlPlane.Machines.Filter(collections.HasDeletionTimestamp).Names(), ", "))

	return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
}

// applyHookToActiveMachines ensures that every active Machine has the hook set.
func applyHookToActiveMachines(ctx context.Context, controlPlane *rke2.ControlPlane, hookAnnotation string) error {
	log := ctrl.LoggerFrom(ctx)

	// Ensure that every active machine has the hook set
	patchHookAnnotation := false

	for _, machine := range controlPlane.Machines.Filter(collections.ActiveMachines) {
		if _, exists := machine.Annotations[hookAnnotation]; !exists {
			log.Info("Applying hook on machine", "hook", hookAnnotation, "machine", machine.Name)
			machine.Annotations[hookAnnotation] = ""
			patchHookAnnotation = true
		}
	}

	if patchHookAnnotation {
		// Patch machine annotations
		if err := controlPlane.PatchMachines(ctx); err != nil {
			return fmt.Errorf("patching machines: %w", err)
		}
	}

	return nil
}

// cleanupHookOnAllMachines removes the hook annotation on every Machine.
func cleanupHookOnAllMachines(ctx context.Context, controlPlane *rke2.ControlPlane, hookAnnotation string) error {
	log := ctrl.LoggerFrom(ctx)

	patchHookAnnotation := false

	for _, machine := range controlPlane.Machines {
		if _, exists := machine.Annotations[hookAnnotation]; exists {
			log.Info("Cleaning up hook from machine", "hook", hookAnnotation, "machine", machine.Name)
			delete(machine.Annotations, hookAnnotation)

			patchHookAnnotation = true
		}
	}

	if patchHookAnnotation {
		// Patch machine annotations
		if err := controlPlane.PatchMachines(ctx); err != nil {
			return fmt.Errorf("patching machines: %w", err)
		}
	}

	return nil
}

func machineHasOtherHooks(machine *clusterv1.Machine, hookPrefix string, hookAnnotation string) bool {
	for k := range machine.Annotations {
		if strings.HasPrefix(k, hookPrefix) && k != hookAnnotation {
			return true
		}
	}

	return false
}

func getDeletingMachineWithHook(ctx context.Context,
	controlPlane *rke2.ControlPlane,
	hookAnnotation string,
) *clusterv1.Machine {
	log := ctrl.LoggerFrom(ctx)
	// Return early, if there is already a deleting Machine without the hook.
	// We are going to wait until this Machine goes away before running the hook on other Machines.
	for _, deletingMachine := range controlPlane.DeletingMachines() {
		if _, exists := deletingMachine.Annotations[hookAnnotation]; !exists {
			log.V(5).Info("Machine does not have hook", "hook", hookAnnotation, "machine", deletingMachine.Name)

			return nil
		}
	}

	// Pick the Machine with the oldest deletionTimestamp to keep this function deterministic / reentrant
	// so we only remove the hook from one Machine at a time.
	deletingMachine := controlPlane.DeletingMachines().OldestDeletionTimestamp()

	// Return early because the Machine controller is not yet waiting for the hook.
	c := conditions.Get(deletingMachine, clusterv1.MachineDeletingCondition)

	// Return early because the Machine controller is not yet waiting for the pre-terminate hook.
	if c == nil || c.Status != metav1.ConditionTrue ||
		(c.Reason != clusterv1.MachineDeletingWaitingForPreTerminateHookReason && c.Reason != clusterv1.MachineDeletingWaitingForPreDrainHookReason) {
		log.V(5).Info("Machine is not waiting on condition", "condition", clusterv1.MachineDeletingCondition)

		return nil
	}

	return deletingMachine
}

func (r *RKE2ControlPlaneReconciler) removeHookAnnotationFromMachine(ctx context.Context, machine *clusterv1.Machine, hookAnnotation string) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Removing hook from Machine", "hook", hookAnnotation, "machine", machine.Name)

	if _, exists := machine.Annotations[hookAnnotation]; !exists {
		// Nothing to do, the annotation is not set (anymore) on the Machine
		return nil
	}

	machineOriginal := machine.DeepCopy()
	delete(machine.Annotations, hookAnnotation)

	if err := r.Patch(ctx, machine, client.MergeFrom(machineOriginal)); err != nil {
		return fmt.Errorf("removing pre-terminate hook from control plane Machine %s: %w", klog.KObj(machine), err)
	}

	return nil
}

func (r *RKE2ControlPlaneReconciler) removePreTerminateHookAnnotationFromMachine(ctx context.Context, machine *clusterv1.Machine) error {
	if _, exists := machine.Annotations[controlplanev1.PreTerminateHookCleanupAnnotation]; !exists {
		// Nothing to do, the annotation is not set (anymore) on the Machine
		return nil
	}

	log := ctrl.LoggerFrom(ctx)
	log.Info("Removing pre-terminate hook from Machine", "hook", controlplanev1.PreTerminateHookCleanupAnnotation, "machine", machine.Name)

	machineOriginal := machine.DeepCopy()
	delete(machine.Annotations, controlplanev1.PreTerminateHookCleanupAnnotation)

	// Mitigating inssue https://github.com/kubernetes-sigs/cluster-api/issues/11591
	machine.Annotations[clusterv1.ExcludeNodeDrainingAnnotation] = trueString
	machine.Annotations[clusterv1.ExcludeWaitForNodeVolumeDetachAnnotation] = trueString

	if err := r.Patch(ctx, machine, client.MergeFrom(machineOriginal)); err != nil {
		return fmt.Errorf("removing pre-terminate hook from control plane Machine %s: %w", klog.KObj(machine), err)
	}

	return nil
}
