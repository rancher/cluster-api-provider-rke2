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

	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	"github.com/rancher/cluster-api-provider-rke2/pkg/rke2"
)

func (r *RKE2ControlPlaneReconciler) tryInPlaceUpdate(
	ctx context.Context,
	controlPlane *rke2.ControlPlane,
	machineToInPlaceUpdate *clusterv1.Machine,
	machineUpToDateResult rke2.UpToDateResult,
) (fallbackToScaleDown bool, _ ctrl.Result, _ error) {
	// 1. Preflight checks for all machines.
	if resultForAllMachines := r.preflightChecks(ctx, controlPlane); !resultForAllMachines.IsZero() {
		if result := r.preflightChecks(ctx, controlPlane, machineToInPlaceUpdate); result.IsZero() {
			return true, ctrl.Result{}, nil
		}

		return false, resultForAllMachines, nil
	}

	// 2. CanUpdateMachine check.
	canUpdate, err := r.canUpdateMachine(ctx, machineToInPlaceUpdate, machineUpToDateResult)
	if err != nil {
		return false, ctrl.Result{}, fmt.Errorf("failed to determine if Machine %s can be updated in-place: %w", klog.KObj(machineToInPlaceUpdate), err)
	}

	if !canUpdate {
		return true, ctrl.Result{}, nil
	}

	return false, ctrl.Result{}, r.triggerInPlaceUpdate(ctx, machineToInPlaceUpdate, machineUpToDateResult)
}
