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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	clientutil "sigs.k8s.io/cluster-api/util/client"
	"sigs.k8s.io/cluster-api/util/hooks"

	"github.com/rancher/cluster-api-provider-rke2/controlplane/internal/util/ssa"
	"github.com/rancher/cluster-api-provider-rke2/pkg/rke2"
)

func (r *RKE2ControlPlaneReconciler) triggerInPlaceUpdate(
	ctx context.Context, machine *clusterv1.Machine,
	machineUpToDateResult rke2.UpToDateResult,
) error {
	log := ctrl.LoggerFrom(ctx).WithValues("Machine", klog.KObj(machine))
	log.Info(fmt.Sprintf("Triggering in-place update for Machine %s", klog.KObj(machine)))

	// Mark Machine for in-place update.
	// Note: Once we write UpdateInProgressAnnotation we will always continue with the in-place update.
	// Note: Intentionally using client.Patch instead of SSA. Otherwise we would have to ensure we preserve
	//       UpdateInProgressAnnotation on existing Machines in RCP and that would lead to race conditions when
	//       the Machine controller tries to remove the annotation and then RCP adds it back.
	if _, ok := machine.Annotations[clusterv1.UpdateInProgressAnnotation]; !ok {
		orig := machine.DeepCopy()
		if machine.Annotations == nil {
			machine.Annotations = map[string]string{}
		}

		machine.Annotations[clusterv1.UpdateInProgressAnnotation] = ""

		if err := r.Patch(ctx, machine, client.MergeFrom(orig)); err != nil {
			return errors.Wrapf(err,
				"failed to trigger in-place update for Machine %s by setting the %s annotation",
				klog.KObj(machine), clusterv1.UpdateInProgressAnnotation,
			)
		}

		// Wait until the cache observed the Machine with UpdateInProgressAnnotation to ensure subsequent reconciles
		// will observe it as well and accordingly don't trigger another in-place update concurrently.
		if err := clientutil.WaitForCacheToBeUpToDate(ctx, r.Client,
			fmt.Sprintf("setting the %s annotation", clusterv1.UpdateInProgressAnnotation),
			machine,
		); err != nil {
			return err
		}
	}

	desiredMachine := machineUpToDateResult.DesiredMachine
	desiredInfraMachine := machineUpToDateResult.DesiredInfraMachine
	desiredRKE2Config := machineUpToDateResult.DesiredRKE2Config

	// Machine cannot be updated in-place if the UpToDate func was not able to provide all objects,
	// e.g. if the InfraMachine or RKE2Config was deleted.
	// Note: As canUpdateMachine also checks these fields for nil this can only happen if the initial
	//      triggerInPlaceUpdate call failed after setting UpdateInProgressAnnotation.
	if desiredInfraMachine == nil {
		return errors.Errorf("failed to complete triggering in-place update for Machine %s, could not compute desired InfraMachine", klog.KObj(machine))
	}

	if desiredRKE2Config == nil {
		return errors.Errorf("failed to complete triggering in-place update for Machine %s, could not compute desired RKE2Config", klog.KObj(machine))
	}

	// Write InfraMachine without the labels & annotations that are written continuously by syncMachines.
	// Note: Let's update InfraMachine first because that is the call that is most likely to fail.
	desiredInfraMachine.SetLabels(nil)
	desiredInfraMachine.SetAnnotations(map[string]string{
		// ClonedFrom annotations are initially written by createInfraMachine and then managedField ownership is
		// removed via ssa.RemoveManagedFieldsForLabelsAndAnnotations.
		// syncMachines is intentionally not updating them as they should be only updated as part
		// of an in-place update here, e.g. for the case where the InfraMachineTemplate was rotated.
		clusterv1.TemplateClonedFromNameAnnotation:      desiredInfraMachine.GetAnnotations()[clusterv1.TemplateClonedFromNameAnnotation],
		clusterv1.TemplateClonedFromGroupKindAnnotation: desiredInfraMachine.GetAnnotations()[clusterv1.TemplateClonedFromGroupKindAnnotation],
		// Machine controller waits for this annotation to exist on Machine and related objects before starting the in-place update.
		clusterv1.UpdateInProgressAnnotation: "",
	})

	if err := ssa.Patch(ctx, r.Client, rke2ManagerName, desiredInfraMachine); err != nil {
		return errors.Wrapf(err, "failed to complete triggering in-place update for Machine %s", klog.KObj(machine))
	}

	// Write RKE2Config without the labels & annotations that are written continuously by syncMachines.
	desiredRKE2Config.Labels = nil

	desiredRKE2Config.Annotations = map[string]string{
		// Machine controller waits for this annotation to exist on Machine and related objects before starting the in-place update.
		clusterv1.UpdateInProgressAnnotation: "",
	}
	if err := ssa.Patch(ctx, r.Client, rke2ManagerName, desiredRKE2Config); err != nil {
		return errors.Wrapf(err, "failed to complete triggering in-place update for Machine %s", klog.KObj(machine))
	}

	// Write Machine.
	if err := ssa.Patch(ctx, r.Client, rke2ManagerName, desiredMachine); err != nil {
		return errors.Wrapf(err, "failed to complete triggering in-place update for Machine %s", klog.KObj(machine))
	}

	// Note: Once we write PendingHooksAnnotation the Machine controller will start with the in-place update.
	// Note: Intentionally using client.Patch (via hooks.MarkAsPending + patchHelper) instead of SSA. Otherwise we would
	//       have to ensure we preserve PendingHooksAnnotation on existing Machines in RCP and that would lead to race
	//       conditions when the Machine controller tries to remove the annotation and RCP adds it back.
	// Note: This call will update the resourceVersion on desiredMachine, so that WaitForCacheToBeUpToDate also considers this change.
	if err := hooks.MarkAsPending(ctx, r.Client, desiredMachine, true, runtimehooksv1.UpdateMachine); err != nil {
		return errors.Wrapf(err, "failed to complete triggering in-place update for Machine %s", klog.KObj(machine))
	}

	log.Info(fmt.Sprintf("Completed triggering in-place update for Machine %s", klog.KObj(machine)))
	r.recorder.Event(machine, corev1.EventTypeNormal, "SuccessfulStartInPlaceUpdate", "Machine starting in-place update")

	// Wait until the cache observed the Machine with PendingHooksAnnotation to ensure subsequent reconciles
	// will observe it as well and won't repeatedly call triggerInPlaceUpdate.
	return clientutil.WaitForCacheToBeUpToDate(ctx, r.Client, "marking the UpdateMachine hook as pending", desiredMachine)
}
