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
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1alpha1"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/rke2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// RKE2ControlPlaneReconciler reconciles a RKE2ControlPlane object
type RKE2ControlPlaneReconciler struct {
	Log logr.Logger
	client.Client
	Scheme                    *runtime.Scheme
	managementClusterUncached rke2.ManagementCluster
	managementCluster         rke2.ManagementCluster
	recorder                  record.EventRecorder
	controller                controller.Controller
}

//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;machinesets;machines;machines/status;machinepools;machinepools/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets;events;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="bootstrap.cluster.x-k8s.io",resources=rke2configs,verbs=get;list;watch;create;patch;delete
// +kubebuilder:rbac:groups="infrastructure.cluster.x-k8s.io",resources=*,verbs=get;list;watch;create;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RKE2ControlPlane object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *RKE2ControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, reterr error) {
	logger := log.FromContext(ctx)
	r.Log = logger
	rcp := &controlplanev1.RKE2ControlPlane{}
	if err := r.Get(ctx, req.NamespacedName, rcp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, err
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, rcp.ObjectMeta)
	if err != nil {
		logger.Error(err, "Failed to retrieve owner Cluster from the API Server")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		logger.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{Requeue: true}, nil
	}

	logger = logger.WithValues("cluster", cluster.Name)

	if annotations.IsPaused(cluster, rcp) {
		logger.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(rcp, r.Client)
	if err != nil {
		logger.Error(err, "Failed to configure the patch helper")
		return ctrl.Result{Requeue: true}, nil
	}

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(rcp, controlplanev1.RKE2ControlPlaneFinalizer) {
		controllerutil.AddFinalizer(rcp, controlplanev1.RKE2ControlPlaneFinalizer)

		// patch and return right away instead of reusing the main defer,
		// because the main defer may take too much time to get cluster status
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{patch.WithStatusObservedGeneration{}}
		if err := patchHelper.Patch(ctx, rcp, patchOpts...); err != nil {
			logger.Error(err, "Failed to patch RKE2ControlPlane to add finalizer")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	defer func() {
		// Always attempt to update status.
		if err := r.updateStatus(ctx, patchHelper, rcp); err != nil {
			logger.Error(err, "Failed to update RKE2ControlPlane Status")
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		// Always attempt to Patch the RKE2ControlPlane object and status after each reconciliation.
		if err := patchRKE2ControlPlane(ctx, patchHelper, rcp); err != nil {
			logger.Error(err, "Failed to patch RKE2ControlPlane")
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		// TODO: remove this as soon as we have a proper remote cluster cache in place.
		// Make rcp to requeue in case status is not ready, so we can check for node status without waiting for a full resync (by default 10 minutes).
		// Only requeue if we are not going in exponential backoff due to error, or if we are not already re-queueing, or if the object has a deletion timestamp.
		if reterr == nil && !res.Requeue && res.RequeueAfter <= 0 && rcp.ObjectMeta.DeletionTimestamp.IsZero() {
			if !rcp.Status.Ready {
				res = ctrl.Result{RequeueAfter: 20 * time.Second}
			}
		}
	}()

	if !rcp.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion reconciliation loop.
		res, err = r.reconcileDelete(ctx, cluster, rcp)
		// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
		// the current cluster because of concurrent access.
		//if errors.Is(err, remote.ErrClusterLocked) {
		//logger.V(5).Info("Requeueing because another worker has the lock on the ClusterCacheTracker")
		//return ctrl.Result{Requeue: true}, nil
		//}
		return res, err
	}

	// Handle normal reconciliation loop.
	res, err = r.reconcileNormal(ctx, cluster, rcp)
	// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
	// the current cluster because of concurrent access.
	//if errors.Is(err, remote.ErrClusterLocked) {
	//	logger.V(5).Info("Requeueing because another worker has the lock on the ClusterCacheTracker")
	//	return ctrl.Result{Requeue: true}, nil
	//}
	return res, err
}

func patchRKE2ControlPlane(ctx context.Context, patchHelper *patch.Helper, rcp *controlplanev1.RKE2ControlPlane) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	conditions.SetSummary(rcp,
		conditions.WithConditions(
			controlplanev1.MachinesReadyCondition,
			controlplanev1.MachinesSpecUpToDateCondition,
			controlplanev1.ResizedCondition,
			controlplanev1.MachinesReadyCondition,
			controlplanev1.AvailableCondition,
			// controlplanev1.CertificatesAvailableCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		rcp,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			controlplanev1.MachinesSpecUpToDateCondition,
			controlplanev1.ResizedCondition,
			controlplanev1.MachinesReadyCondition,
			controlplanev1.AvailableCondition,
		}},
		patch.WithStatusObservedGeneration{},
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RKE2ControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1.RKE2ControlPlane{}).
		//Owns(&clusterv1.Machine{}).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	err = c.Watch(
		&source.Kind{Type: &clusterv1.Cluster{}},
		handler.EnqueueRequestsFromMapFunc(r.ClusterToRKE2ControlPlane),
		//predicates.ClusterUnpausedAndInfrastructureReady(r.Log),
	)
	if err != nil {
		return errors.Wrap(err, "failed adding Watch for Clusters to controller manager")
	}
	//r.Scheme = mgr.GetScheme()
	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("rke2-control-plane-controller")

	if r.managementCluster == nil {
		r.managementCluster = &rke2.Management{Client: r.Client}
	}
	if r.managementClusterUncached == nil {
		r.managementClusterUncached = &rke2.Management{Client: mgr.GetAPIReader()}
	}
	return nil
}

func (r *RKE2ControlPlaneReconciler) updateStatus(ctx context.Context, patchHelper *patch.Helper, rcp *controlplanev1.RKE2ControlPlane) error {
	return nil
}

func (r *RKE2ControlPlaneReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, rcp *controlplanev1.RKE2ControlPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconcile RKE2 Control Plane")

	// Wait for the cluster infrastructure to be ready before creating machines
	if !cluster.Status.InfrastructureReady {
		logger.Info("Cluster infrastructure is not ready yet")
		return ctrl.Result{}, nil
	}

	// If ControlPlaneEndpoint is not set, return early
	if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		logger.Info("Cluster does not yet have a ControlPlaneEndpoint defined")
		return ctrl.Result{}, nil
	}

	// Generate Cluster Kubeconfig if needed
	if result, err := r.reconcileKubeconfig(ctx, util.ObjectKey(cluster), cluster.Spec.ControlPlaneEndpoint, rcp); err != nil {
		logger.Error(err, "failed to reconcile Kubeconfig")
		return result, err
	}

	controlPlaneMachines, err := r.managementClusterUncached.GetMachinesForCluster(ctx, util.ObjectKey(cluster), collections.ControlPlaneMachines(cluster.Name))
	if err != nil {
		logger.Error(err, "failed to retrieve control plane machines for cluster")
		return ctrl.Result{}, err
	}
	//logger.V(5).Info()
	//adoptableMachines := controlPlaneMachines.Filter(machinefilters.AdoptableControlPlaneMachines(cluster.Name))
	//if len(adoptableMachines) > 0 {
	//// We adopt the Machines and then wait for the update event for the ownership reference to re-queue them so the cache is up-to-date
	//err = r.adoptMachines(ctx, rcp, adoptableMachines, cluster)
	//return ctrl.Result{}, err
	//}

	logger.V(5).Info("Checking on Reconciler 2")

	ownedMachines := controlPlaneMachines.Filter(collections.OwnedMachines(rcp))
	if len(ownedMachines) != len(controlPlaneMachines) {
		logger.Info("Not all control plane machines are owned by this RKE2ControlPlane, refusing to operate in mixed management mode")
		return ctrl.Result{}, nil
	}

	controlPlane, err := rke2.NewControlPlane(ctx, r.Client, cluster, rcp, ownedMachines)
	if err != nil {
		logger.Error(err, "failed to initialize control plane")
		return ctrl.Result{}, err
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	conditions.SetAggregate(controlPlane.RCP, controlplanev1.MachinesReadyCondition, ownedMachines.ConditionGetters(), conditions.AddSourceRef(), conditions.WithStepCounterIf(false))

	// Updates conditions reporting the status of static pods and the status of the etcd cluster.
	// NOTE: Conditions reporting RCP operation progress like e.g. Resized or SpecUpToDate are inlined with the rest of the execution.
	if result, err := r.reconcileControlPlaneConditions(ctx, controlPlane); err != nil || !result.IsZero() {
		logger.Error(err, "failed to reconcile Control Plane conditions")
		return result, err
	}

	// Control plane machines rollout due to configuration changes (e.g. upgrades) takes precedence over other operations.
	needRollout := controlPlane.MachinesNeedingRollout()
	switch {
	case len(needRollout) > 0:
		logger.Info("Rolling out Control Plane machines", "needRollout", needRollout.Names())
		conditions.MarkFalse(controlPlane.RCP, controlplanev1.MachinesSpecUpToDateCondition, controlplanev1.RollingUpdateInProgressReason, clusterv1.ConditionSeverityWarning, "Rolling %d replicas with outdated spec (%d replicas up to date)", len(needRollout), len(controlPlane.Machines)-len(needRollout))
		return r.upgradeControlPlane(ctx, cluster, rcp, controlPlane, needRollout)
	default:
		// make sure last upgrade operation is marked as completed.
		// NOTE: we are checking the condition already exists in order to avoid to set this condition at the first
		// reconciliation/before a rolling upgrade actually starts.
		if conditions.Has(controlPlane.RCP, controlplanev1.MachinesSpecUpToDateCondition) {
			conditions.MarkTrue(controlPlane.RCP, controlplanev1.MachinesSpecUpToDateCondition)
		}
	}

	// If we've made it this far, we can assume that all ownedMachines are up to date
	numMachines := len(ownedMachines)
	desiredReplicas := int(*rcp.Spec.Replicas)

	switch {
	// We are creating the first replica
	case numMachines < desiredReplicas && numMachines == 0:
		// Create new Machine w/ init
		logger.Info("Initializing control plane", "Desired", desiredReplicas, "Existing", numMachines)
		conditions.MarkFalse(controlPlane.RCP, controlplanev1.AvailableCondition, controlplanev1.WaitingForRKE2ServerReason, clusterv1.ConditionSeverityInfo, "")
		return r.initializeControlPlane(ctx, cluster, rcp, controlPlane)
	// We are scaling up
	case numMachines < desiredReplicas && numMachines > 0:
		// Create a new Machine w/ join
		logger.Info("Scaling up control plane", "Desired", desiredReplicas, "Existing", numMachines)
		return r.scaleUpControlPlane(ctx, cluster, rcp, controlPlane)
	// We are scaling down
	case numMachines > desiredReplicas:
		logger.Info("Scaling down control plane", "Desired", desiredReplicas, "Existing", numMachines)
		// The last parameter (i.e. machines needing to be rolled out) should always be empty here.
		return r.scaleDownControlPlane(ctx, cluster, rcp, controlPlane, collections.Machines{})
	}

	return ctrl.Result{}, nil
}

func (r *RKE2ControlPlaneReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, rcp *controlplanev1.RKE2ControlPlane) (res ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	// TODO: Improve this part once there are dependencies on the Control Plane Object!
	var ok bool = true
	for _, finalizer := range rcp.GetObjectMeta().GetFinalizers() {
		if ok {
			ok = controllerutil.RemoveFinalizer(rcp, finalizer)
		}
	}
	if !ok {
		logger.Info("unable to remove all finalizers")
		err = fmt.Errorf("unable to remove all finalizers")
		res = ctrl.Result{}
		return
	}

	if err = r.Delete(ctx, rcp); err != nil {
		res = ctrl.Result{RequeueAfter: 2 * time.Minute}
		return
	}

	return ctrl.Result{}, nil
}

func (r *RKE2ControlPlaneReconciler) reconcileKubeconfig(
	ctx context.Context,
	namespacedName types.NamespacedName,
	endpoint clusterv1.APIEndpoint,
	rcp *controlplanev1.RKE2ControlPlane) (ctrl.Result, error) {

	return ctrl.Result{}, nil
}

// reconcileControlPlaneConditions is responsible of reconciling conditions reporting the status of static pods and
// the status of the etcd cluster.
func (r *RKE2ControlPlaneReconciler) reconcileControlPlaneConditions(ctx context.Context, controlPlane *rke2.ControlPlane) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If the cluster is not yet initialized, there is no way to connect to the workload cluster and fetch information
	// for updating conditions. Return early.
	if !controlPlane.RCP.Status.Initialized {
		return ctrl.Result{}, nil
	}

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(controlPlane.Cluster))
	if err != nil {
		logger.Info("ControlPlane not yet initialized")
		return ctrl.Result{}, errors.Wrap(err, "cannot get remote client to workload cluster")
	}

	// Update conditions status
	workloadCluster.UpdateAgentConditions(ctx, controlPlane)
	workloadCluster.UpdateEtcdConditions(ctx, controlPlane)

	// Patch machines with the updated conditions.
	if err := controlPlane.PatchMachines(ctx); err != nil {
		return ctrl.Result{}, err
	}

	// RCP will be patched at the end of Reconcile to reflect updated conditions, so we can return now.
	return ctrl.Result{}, nil
}

func (r *RKE2ControlPlaneReconciler) upgradeControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
	controlPlane *rke2.ControlPlane,
	machinesRequireUpgrade collections.Machines,
) (ctrl.Result, error) {
	logger := controlPlane.Logger()

	// If the cluster is not yet initialized, there is no way to connect to the workload cluster and fetch information
	// for updating conditions. Return early.
	if !rcp.Status.Initialized {
		logger.Info("ControlPlane not yet initialized")
		return ctrl.Result{}, nil
	}

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		logger.Error(err, "failed to get remote client for workload cluster", "cluster key", util.ObjectKey(cluster))
		return ctrl.Result{}, err
	}

	status, err := workloadCluster.ClusterStatus(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	if status.Nodes <= *rcp.Spec.Replicas {
		// scaleUp ensures that we don't continue scaling up while waiting for Machines to have NodeRefs
		return r.scaleUpControlPlane(ctx, cluster, rcp, controlPlane)
	}
	return r.scaleDownControlPlane(ctx, cluster, rcp, controlPlane, machinesRequireUpgrade)
}

// ClusterToRKE2ControlPlane is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for RKE2ControlPlane based on updates to a Cluster.
func (r *RKE2ControlPlaneReconciler) ClusterToRKE2ControlPlane(o client.Object) []ctrl.Request {
	c, ok := o.(*clusterv1.Cluster)
	if !ok {
		r.Log.Error(nil, fmt.Sprintf("Expected a Cluster but got a %T", o))
		return nil
	}

	controlPlaneRef := c.Spec.ControlPlaneRef
	if controlPlaneRef != nil && controlPlaneRef.Kind == "RKE2ControlPlane" {
		return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}}}
	}

	return nil
}
