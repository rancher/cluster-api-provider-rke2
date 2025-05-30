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
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	capikubeconfig "sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	"github.com/rancher/cluster-api-provider-rke2/controlplane/internal/contract"
	"github.com/rancher/cluster-api-provider-rke2/controlplane/internal/util/ssa"
	"github.com/rancher/cluster-api-provider-rke2/pkg/kubeconfig"
	"github.com/rancher/cluster-api-provider-rke2/pkg/registration"
	"github.com/rancher/cluster-api-provider-rke2/pkg/rke2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/secret"
	rke2util "github.com/rancher/cluster-api-provider-rke2/pkg/util"
)

const (
	// rke2ManagerName is the name of the RKE2 manager deployment.
	rke2ManagerName = "rke2controlplane"

	// rke2ControlPlaneKind is the kind of the RKE2 control plane.
	rke2ControlPlaneKind = "RKE2ControlPlane"

	// dependentCertRequeueAfter is how long to wait before checking again to see if
	// dependent certificates have been created.
	dependentCertRequeueAfter = 30 * time.Second

	// DefaultRequeueTime is the default requeue time for the controller.
	DefaultRequeueTime = 20 * time.Second
)

// RKE2ControlPlaneReconciler reconciles a RKE2ControlPlane object.
type RKE2ControlPlaneReconciler struct {
	Log logr.Logger
	client.Client
	Scheme *runtime.Scheme

	SecretCachingClient client.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	managementClusterUncached rke2.ManagementCluster
	managementCluster         rke2.ManagementCluster
	recorder                  record.EventRecorder
	controller                controller.Controller
	ssaCache                  ssa.Cache
}

//nolint:lll
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;machinesets;machines;machines/status;machinepools;machinepools/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets;events;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="bootstrap.cluster.x-k8s.io",resources=rke2configs,verbs=get;list;watch;create;patch;delete
// +kubebuilder:rbac:groups="infrastructure.cluster.x-k8s.io",resources=*,verbs=get;list;watch;create;patch;delete
// +kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RKE2ControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, reterr error) {
	logger := log.FromContext(ctx)
	r.Log = logger
	rcp := &controlplanev1.RKE2ControlPlane{}

	if err := r.Get(ctx, req.NamespacedName, rcp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
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
			return ctrl.Result{}, errors.Wrapf(err, "failed to add finalizer")
		}

		return ctrl.Result{Requeue: true}, nil
	}

	defer func() {
		// Always attempt to update status.
		if err := r.updateStatus(ctx, rcp, cluster); err != nil {
			var connFailure *rke2.RemoteClusterConnectionError
			if errors.As(err, &connFailure) {
				logger.Info("Could not connect to workload cluster to fetch status", "err", err.Error())
			} else {
				logger.Error(err, "Failed to update RKE2ControlPlane Status")
				reterr = kerrors.NewAggregate([]error{reterr, err})
			}
		}

		// Always attempt to Patch the RKE2ControlPlane object and status after each reconciliation.
		if err := patchRKE2ControlPlane(ctx, patchHelper, rcp); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		// Make rcp to requeue in case status is not ready, so we can check for node
		// status without waiting for a full resync (by default 10 minutes).
		// Only requeue if we are not going in exponential backoff due to error,
		// or if we are not already re-queueing, or if the object has a deletion timestamp.
		if reterr == nil && !res.Requeue && res.RequeueAfter <= 0 && rcp.DeletionTimestamp.IsZero() {
			if !rcp.Status.Ready {
				res = ctrl.Result{RequeueAfter: DefaultRequeueTime}
			}
		}
	}()

	if !rcp.DeletionTimestamp.IsZero() {
		// Handle deletion reconciliation loop.
		res, err = r.reconcileDelete(ctx, cluster, rcp)

		return res, err
	}

	updated := false

	// Backfill MachineTemplate.InfrastructureRef if missing but legacy Spec.InfrastructureRef exists
	if rcp.Spec.InfrastructureRef.Name != "" && rcp.Spec.MachineTemplate.InfrastructureRef.Name == "" {
		rcp.Spec.MachineTemplate.InfrastructureRef = rcp.Spec.InfrastructureRef
		updated = true
	}

	// Ensure MachineTemplate.InfrastructureRef.Namespace is set
	if rcp.Spec.MachineTemplate.InfrastructureRef.Name != "" && rcp.Spec.MachineTemplate.InfrastructureRef.Namespace == "" {
		rcp.Spec.MachineTemplate.InfrastructureRef.Namespace = rcp.Namespace
		updated = true
	}

	if updated {
		if err := patchHelper.Patch(ctx, rcp); err != nil {
			logger.Error(err, "Failed to patch RKE2ControlPlane during backfill")
			// If patching fails, we return an error to avoid re-queuing the object.
			return ctrl.Result{}, err
		}
		// Log the backfill operation.
		logger.Info("Backfilled missing RKE2ControlPlane fields from legacy format", "rcp", klog.KObj(rcp))
		// Requeue to ensure the controller reprocesses the object with the updated fields.
		return ctrl.Result{Requeue: true}, nil
	}

	// Handle normal reconciliation loop.
	res, err = r.reconcileNormal(ctx, cluster, rcp)

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
func (r *RKE2ControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, clientQPS float32, clientBurst, concurrency int) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1.RKE2ControlPlane{}).
		Owns(&clusterv1.Machine{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: concurrency,
		}).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	err = c.Watch(
		source.Kind[client.Object](mgr.GetCache(), &clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc((r.ClusterToRKE2ControlPlane(ctx))),
		),
	)
	if err != nil {
		return errors.Wrap(err, "failed adding Watch for Clusters to controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("rke2-control-plane-controller")
	r.ssaCache = ssa.NewCache("rke2-control-plane")

	// Set up a clusterCache to provide to controllers
	// requiring a connection to a remote cluster
	clusterCache, err := clustercache.SetupWithManager(ctx, mgr, clustercache.Options{
		SecretClient: r.SecretCachingClient,
		Cache: clustercache.CacheOptions{
			Indexes: []clustercache.CacheOptionsIndex{clustercache.NodeProviderIDIndex},
		},
		Client: clustercache.ClientOptions{
			QPS:       clientQPS,
			Burst:     clientBurst,
			UserAgent: remote.DefaultClusterAPIUserAgent("rke2-control-plane-controller"),
			Cache: clustercache.ClientCacheOptions{
				DisableFor: []client.Object{
					// Don't cache ConfigMaps & Secrets.
					&corev1.ConfigMap{},
					&corev1.Secret{},
					// Don't cache Pods & DaemonSets (we get/list them e.g. during drain).
					&corev1.Pod{},
					&appsv1.DaemonSet{},
					// Don't cache PersistentVolumes and VolumeAttachments (we get/list them e.g. during wait for volumes to detach)
					&storagev1.VolumeAttachment{},
					&corev1.PersistentVolume{},
				},
			},
		},
	}, controller.Options{})
	if err != nil {
		return errors.Wrap(err, "unable to create cluster cache tracker")
	}

	if r.managementCluster == nil {
		r.managementCluster = &rke2.Management{
			Client:              r.Client,
			SecretCachingClient: r.SecretCachingClient,
			ClusterCache:        clusterCache,
		}
	}

	if r.managementClusterUncached == nil {
		r.managementClusterUncached = &rke2.Management{Client: mgr.GetClient()}
	}

	return nil
}

// ClusterToRKE2ControlPlane is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for RKE2ControlPlane based on updates to a Cluster.
func (r *RKE2ControlPlaneReconciler) ClusterToRKE2ControlPlane(ctx context.Context) handler.MapFunc {
	log := log.FromContext(ctx)

	return func(_ context.Context, o client.Object) []ctrl.Request {
		c, ok := o.(*clusterv1.Cluster)
		if !ok {
			log.Error(nil, fmt.Sprintf("Expected a Cluster but got a %T", o))

			return nil
		}

		controlPlaneRef := c.Spec.ControlPlaneRef
		if controlPlaneRef != nil && controlPlaneRef.Kind == "RKE2ControlPlane" {
			return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}}}
		}

		return nil
	}
}

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
		util.ObjectKey(cluster),
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

	rcp.Status.UpdatedReplicas = rke2util.SafeInt32(len(controlPlane.UpToDateMachines(ctx)))
	replicas := rke2util.SafeInt32(len(ownedMachines))
	desiredReplicas := *rcp.Spec.Replicas

	// set basic data that does not require interacting with the workload cluster
	// ReadyReplicas and UnavailableReplicas are set in case the function returns before updating them
	rcp.Status.Replicas = replicas
	rcp.Status.ReadyReplicas = 0
	rcp.Status.UnavailableReplicas = replicas

	// Return early if the deletion timestamp is set, because we don't want to try to connect to the workload cluster
	// and we don't want to report resize condition (because it is set to deleting into reconcile delete).
	if !rcp.DeletionTimestamp.IsZero() {
		return nil
	}

	switch {
	// We are scaling up
	case replicas < desiredReplicas:
		conditions.MarkFalse(
			rcp,
			controlplanev1.ResizedCondition,
			controlplanev1.ScalingUpReason,
			clusterv1.ConditionSeverityWarning,
			"Scaling up control plane to %d replicas (actual %d)",
			desiredReplicas,
			replicas)

	// We are scaling down
	case replicas > desiredReplicas:
		conditions.MarkFalse(
			rcp,
			controlplanev1.ResizedCondition,
			controlplanev1.ScalingDownReason,
			clusterv1.ConditionSeverityWarning,
			"Scaling down control plane to %d replicas (actual %d)",
			desiredReplicas,
			replicas)

	default:
		// make sure last resize operation is marked as completed.
		// NOTE: we are checking the number of machines ready so we report resize completed only when the machines
		// are actually provisioned (vs reporting completed immediately after the last machine object is created).
		if rke2util.SafeInt32(len(readyMachines)) == replicas {
			conditions.MarkTrue(rcp, controlplanev1.ResizedCondition)
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

	rcp.Status.ReadyReplicas = rke2util.SafeInt32(len(readyMachines))
	rcp.Status.UnavailableReplicas = replicas - rcp.Status.ReadyReplicas

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
		rcp.Status.Initialized = true
	}

	if len(ownedMachines) == 0 || len(readyMachines) == 0 {
		logger.Info(fmt.Sprintf("No Control Plane Machines exist or are ready for RKE2ControlPlane %s/%s", rcp.Namespace, rcp.Name))

		return nil
	}

	availableCPMachines := readyMachines

	registrationmethod, err := registration.NewRegistrationMethod(string(rcp.Spec.RegistrationMethod))
	if err != nil {
		logger.Error(err, "Failed to get node registration method")

		return fmt.Errorf("getting node registration method: %w", err)
	}

	validIPAddresses, err := registrationmethod(cluster, rcp, availableCPMachines)
	if err != nil {
		logger.Error(err, "Failed to get registration addresses")

		return fmt.Errorf("getting registration addresses: %w", err)
	}

	rcp.Status.AvailableServerIPs = validIPAddresses
	if len(rcp.Status.AvailableServerIPs) == 0 {
		return errors.New("some Control Plane machines exist and are ready but they have no IP Address available")
	}

	if len(readyMachines) == len(ownedMachines) {
		rcp.Status.Ready = true
	}

	conditions.MarkTrue(rcp, controlplanev1.AvailableCondition)

	lowestVersion := controlPlane.Machines.LowestVersion()
	if lowestVersion != nil {
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

	logger.Info("Successfully updated RKE2ControlPlane status", "namespace", rcp.Namespace, "name", rcp.Name)

	return nil
}

func (r *RKE2ControlPlaneReconciler) reconcileNormal(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconcile RKE2 Control Plane")

	// Wait for the cluster infrastructure to be ready before creating machines
	if !cluster.Status.InfrastructureReady {
		logger.Info("Cluster infrastructure is not ready yet")

		return ctrl.Result{}, nil
	}

	certificates := secret.NewCertificatesForInitialControlPlane()
	if _, found := rcp.Annotations[controlplanev1.LegacyRKE2ControlPlane]; found {
		certificates = secret.NewCertificatesForLegacyControlPlane()
	}

	controllerRef := metav1.NewControllerRef(rcp, controlplanev1.GroupVersion.WithKind("RKE2ControlPlane"))

	if err := certificates.LookupOrGenerate(ctx, r.Client, util.ObjectKey(cluster), *controllerRef); err != nil {
		logger.Error(err, "unable to lookup or create cluster certificates")
		conditions.MarkFalse(
			rcp, controlplanev1.CertificatesAvailableCondition,
			controlplanev1.CertificatesGenerationFailedReason,
			clusterv1.ConditionSeverityWarning, "%s", err.Error())

		return ctrl.Result{}, err
	}

	conditions.MarkTrue(rcp, controlplanev1.CertificatesAvailableCondition)

	// If ControlPlaneEndpoint is not set, return early
	if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		logger.Info("Cluster does not yet have a ControlPlaneEndpoint defined")

		return ctrl.Result{}, nil
	}

	// Generate Cluster Kubeconfig if needed
	if result, err := r.reconcileKubeconfig(
		ctx,
		util.ObjectKey(cluster),
		cluster.Spec.ControlPlaneEndpoint,
		rcp); err != nil {
		logger.Error(err, "failed to reconcile Kubeconfig")

		return result, err
	}

	controlPlaneMachines, err := r.managementClusterUncached.GetMachinesForCluster(
		ctx,
		util.ObjectKey(cluster),
		collections.ControlPlaneMachines(cluster.Name))
	if err != nil {
		logger.Error(err, "failed to retrieve control plane machines for cluster")

		return ctrl.Result{}, err
	}

	ownedMachines := controlPlaneMachines.Filter(collections.OwnedMachines(rcp))
	if len(ownedMachines) != len(controlPlaneMachines) {
		logger.Info("Not all control plane machines are owned by this RKE2ControlPlane, refusing to operate in mixed management mode") //nolint:lll

		return ctrl.Result{}, nil
	}

	controlPlane, err := rke2.NewControlPlane(ctx, r.managementCluster, r.Client, cluster, rcp, ownedMachines)
	if err != nil {
		logger.Error(err, "failed to initialize control plane")

		return ctrl.Result{}, err
	}

	if err := controlPlane.ReconcileExternalReference(ctx, r.Client); err != nil {
		logger.Error(err, "Could not reconcile external reference")

		return ctrl.Result{}, fmt.Errorf("reconciling external reference: %w", err)
	}

	if err := r.syncMachines(ctx, controlPlane); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to sync Machines")
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	conditions.SetAggregate(controlPlane.RCP, controlplanev1.MachinesReadyCondition,
		ownedMachines.ConditionGetters(),
		conditions.AddSourceRef(),
		conditions.WithStepCounterIf(false))

	// Updates conditions reporting the status of static pods and the status of the etcd cluster.
	// NOTE: Conditions reporting RCP operation progress like e.g. Resized or SpecUpToDate are inlined with the rest of the execution.
	if result, err := r.reconcileControlPlaneConditions(ctx, controlPlane); err != nil || !result.IsZero() {
		logger.Error(err, "failed to reconcile Control Plane conditions")

		return result, err
	}

	// Ensures the number of etcd members is in sync with the number of machines/nodes.
	// NOTE: This is usually required after a machine deletion.
	if err := r.reconcileEtcdMembers(ctx, controlPlane); err != nil {
		return ctrl.Result{}, err
	}

	if result, err := r.reconcilePreTerminateHook(ctx, controlPlane); err != nil || !result.IsZero() {
		return result, err
	}

	// Reconcile unhealthy machines by triggering deletion and requeue if it is considered safe to remediate,
	// otherwise continue with the other RCP operations.
	if result, err := r.reconcileUnhealthyMachines(ctx, controlPlane); err != nil || !result.IsZero() {
		return result, err
	}

	// Control plane machines rollout due to configuration changes (e.g. upgrades) takes precedence over other operations.
	needRollout := controlPlane.MachinesNeedingRollout(ctx)

	switch {
	case len(needRollout) > 0:
		logger.Info("Rolling out Control Plane machines", "needRollout", needRollout.Names())
		conditions.MarkFalse(controlPlane.RCP,
			controlplanev1.MachinesSpecUpToDateCondition,
			controlplanev1.RollingUpdateInProgressReason,
			clusterv1.ConditionSeverityWarning,
			"Rolling %d replicas with outdated spec (%d replicas up to date)",
			len(needRollout),
			len(controlPlane.Machines)-len(needRollout))

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
		conditions.MarkFalse(controlPlane.RCP,
			controlplanev1.AvailableCondition,
			controlplanev1.WaitingForRKE2ServerReason,
			clusterv1.ConditionSeverityInfo, "")

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

// reconcileEtcdMembers ensures the number of etcd members is in sync with the number of machines/nodes.
// This is usually required after a machine deletion.
//
// NOTE: this func uses RKE2ControlPlane conditions, it is required to call reconcileControlPlaneConditions before this.
func (r *RKE2ControlPlaneReconciler) reconcileEtcdMembers(ctx context.Context, controlPlane *rke2.ControlPlane) error {
	log := ctrl.LoggerFrom(ctx)

	// If there is no RKE-owned control-plane machines, then control-plane has not been initialized yet.
	if controlPlane.Machines.Len() == 0 {
		return nil
	}

	if _, found := controlPlane.RCP.Annotations[controlplanev1.LegacyRKE2ControlPlane]; found {
		log.Info("Etcd membership disabled, found controlplane.cluster.x-k8s.io/legacy annotation")

		return nil
	}

	// Collect all the node names.
	nodeNames := []string{}

	for _, machine := range controlPlane.Machines {
		if machine.Status.NodeRef == nil {
			// If there are provisioning machines (machines without a node yet), return.
			return nil
		}

		nodeNames = append(nodeNames, machine.Status.NodeRef.Name)
	}

	// Potential inconsistencies between the list of members and the list of machines/nodes are
	// surfaced using the EtcdClusterHealthyCondition; if this condition is true, meaning no inconsistencies exists, return early.
	if conditions.IsTrue(controlPlane.RCP, controlplanev1.EtcdClusterHealthyCondition) {
		return nil
	}

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		// Failing at connecting to the workload cluster can mean workload cluster is unhealthy for a variety of reasons such as etcd quorum loss.
		return errors.Wrap(err, "cannot get remote client to workload cluster")
	}

	parsedVersion, err := semver.ParseTolerant(controlPlane.RCP.GetDesiredVersion())
	if err != nil {
		return errors.Wrapf(err, "failed to parse kubernetes version %q", controlPlane.RCP.GetDesiredVersion())
	}

	removedMembers, err := workloadCluster.ReconcileEtcdMembers(ctx, nodeNames, parsedVersion)
	if err != nil {
		return errors.Wrap(err, "failed attempt to reconcile etcd members")
	}

	if len(removedMembers) > 0 {
		log.Info("Etcd members without nodes removed from the cluster", "members", removedMembers)
	}

	return nil
}

func (r *RKE2ControlPlaneReconciler) reconcileDelete(ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
) (res ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	// Gets all machines, not just control plane machines.
	allMachines, err := r.managementCluster.GetMachinesForCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, err
	}

	ownedMachines := allMachines.Filter(collections.OwnedMachines(rcp))

	// If no control plane machines remain, remove the finalizer
	if len(ownedMachines) == 0 {
		// If the legacy finalizer is present, remove it.
		if controllerutil.ContainsFinalizer(rcp, controlplanev1.RKE2ControlPlaneLegacyFinalizer) {
			controllerutil.RemoveFinalizer(rcp, controlplanev1.RKE2ControlPlaneLegacyFinalizer)
		}

		controllerutil.RemoveFinalizer(rcp, controlplanev1.RKE2ControlPlaneFinalizer)

		return ctrl.Result{}, nil
	}

	controlPlane, err := rke2.NewControlPlane(ctx, r.managementCluster, r.Client, cluster, rcp, ownedMachines)
	if err != nil {
		logger.Error(err, "failed to initialize control plane")

		return ctrl.Result{}, err
	}

	// Updates conditions reporting the status of static pods and the status of the etcd cluster.
	// NOTE: Ignoring failures given that we are deleting
	if _, err := r.reconcileControlPlaneConditions(ctx, controlPlane); err != nil {
		logger.Info("failed to reconcile conditions", "error", err.Error())
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	// However, during delete we are hiding the counter (1 of x) because it does not make sense given that
	// all the machines are deleted in parallel.
	conditions.SetAggregate(rcp,
		controlplanev1.MachinesReadyCondition,
		ownedMachines.ConditionGetters(),
		conditions.AddSourceRef(),
		conditions.WithStepCounterIf(false))

	// Verify that only control plane machines remain
	if len(allMachines) != len(ownedMachines) {
		logger.Info("Waiting for worker nodes to be deleted first")
		conditions.MarkFalse(rcp,
			controlplanev1.ResizedCondition,
			clusterv1.DeletingReason,
			clusterv1.ConditionSeverityInfo,
			"Waiting for worker nodes to be deleted first")

		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// Delete control plane machines in parallel
	machinesToDelete := ownedMachines

	var errs []error

	for i := range machinesToDelete {
		m := machinesToDelete[i]
		logger := logger.WithValues("machine", m)

		// During RKE2CP deletion we don't care about forwarding etcd leadership or removing etcd members.
		// So we are removing the pre-terminate hook.
		// This is important because when deleting RKE2CP we will delete all members of etcd and it's not possible
		// to forward etcd leadership without any member left after we went through the Machine deletion.
		// Also in this case the reconcileDelete code of the Machine controller won't execute Node drain
		// and wait for volume detach.
		if err := r.removePreTerminateHookAnnotationFromMachine(ctx, m); err != nil {
			errs = append(errs, err)

			continue
		}

		if !m.DeletionTimestamp.IsZero() {
			// Nothing to do, Machine already has deletionTimestamp set.
			continue
		}

		if err := r.Delete(ctx, machinesToDelete[i]); err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to cleanup owned machine")
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		err := kerrors.NewAggregate(errs)
		r.recorder.Eventf(rcp, corev1.EventTypeWarning, "FailedDelete",
			"Failed to delete control plane Machines for cluster %s/%s control plane: %v", cluster.Namespace, cluster.Name, err)

		return ctrl.Result{}, err
	}

	logger.Info("Waiting for control plane Machines to not exist anymore")

	conditions.MarkFalse(rcp, controlplanev1.ResizedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")

	return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
}

func (r *RKE2ControlPlaneReconciler) reconcileKubeconfig(
	ctx context.Context,
	clusterName client.ObjectKey,
	endpoint clusterv1.APIEndpoint,
	rcp *controlplanev1.RKE2ControlPlane,
) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)
	if endpoint.IsZero() {
		logger.V(5).Info("API Endpoint not yet known")

		return ctrl.Result{RequeueAfter: DefaultRequeueTime}, nil
	}

	controllerOwnerRef := *metav1.NewControllerRef(rcp, controlplanev1.GroupVersion.WithKind("RKE2ControlPlane"))
	configSecret, err := secret.GetFromNamespacedName(ctx, r.Client, clusterName, secret.Kubeconfig)

	switch {
	case apierrors.IsNotFound(err):
		logger.Info("Kubeconfig Secret not found, creating a new one")

		createErr := kubeconfig.CreateSecretWithOwner(
			ctx,
			r.Client,
			clusterName,
			endpoint.String(),
			controllerOwnerRef,
		)

		if errors.Is(createErr, kubeconfig.ErrDependentCertificateNotFound) {
			logger.Error(createErr, "Could not find Secret CA to create Kubeconfig Secret, requeuing...")

			return ctrl.Result{RequeueAfter: dependentCertRequeueAfter}, nil
		}
		// always return if we have just created in order to skip rotation checks
		return ctrl.Result{}, createErr

	case err != nil:
		return ctrl.Result{}, errors.Wrap(err, "failed to retrieve kubeconfig Secret")
	}

	// only do rotation on owned secrets
	if !util.IsControlledBy(configSecret, rcp) {
		logger.Info("Kubeconfig Secret not controlled by RKE2ControlPlane, nothing to do")

		return ctrl.Result{}, nil
	}

	needsRotation, err := capikubeconfig.NeedsClientCertRotation(configSecret, certs.ClientCertificateRenewalDuration)
	if err != nil {
		return ctrl.Result{}, err
	}

	if needsRotation {
		logger.Info("Rotating kubeconfig secret")

		if err := kubeconfig.UpdateSecret(ctx, r.Client, clusterName, endpoint.String(), configSecret); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to regenerate kubeconfig")
		}
	}

	return ctrl.Result{}, nil
}

// reconcileControlPlaneConditions is responsible of reconciling conditions reporting the status of static pods and
// the status of the etcd cluster.
func (r *RKE2ControlPlaneReconciler) reconcileControlPlaneConditions(
	ctx context.Context, controlPlane *rke2.ControlPlane,
) (res ctrl.Result, retErr error) {
	logger := log.FromContext(ctx)

	readyCPMachines := controlPlane.Machines.Filter(collections.IsReady())

	if readyCPMachines.Len() == 0 {
		controlPlane.RCP.Status.Initialized = false
		controlPlane.RCP.Status.Ready = false
		controlPlane.RCP.Status.ReadyReplicas = 0
		controlPlane.RCP.Status.AvailableServerIPs = nil
		conditions.MarkFalse(
			controlPlane.RCP,
			controlplanev1.AvailableCondition,
			controlplanev1.WaitingForRKE2ServerReason,
			clusterv1.ConditionSeverityInfo, "")
		conditions.MarkFalse(
			controlPlane.RCP,
			controlplanev1.MachinesReadyCondition,
			controlplanev1.WaitingForRKE2ServerReason,
			clusterv1.ConditionSeverityInfo, "")
	}

	// If the cluster is not yet initialized, there is no way to connect to the workload cluster and fetch information
	// for updating conditions. Return early.
	if !controlPlane.RCP.Status.Initialized {
		return ctrl.Result{}, nil
	}

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		logger.Error(err, "Failed to get remote client for workload cluster", "cluster key", util.ObjectKey(controlPlane.Cluster))

		return ctrl.Result{}, fmt.Errorf("getting workload cluster: %w", err)
	}

	defer func() {
		// Always attempt to Patch the Machine conditions after each reconcile.
		if err := controlPlane.PatchMachines(ctx); err != nil {
			retErr = kerrors.NewAggregate([]error{retErr, err})
		}
	}()

	if err := workloadCluster.InitWorkload(ctx, controlPlane); err != nil {
		logger.Error(err, "Unable to initialize workload cluster")

		return ctrl.Result{}, err
	}

	// Update conditions status
	workloadCluster.UpdateAgentConditions(controlPlane)
	workloadCluster.UpdateEtcdConditions(controlPlane)

	// Patch nodes metadata
	if err := workloadCluster.UpdateNodeMetadata(ctx, controlPlane); err != nil {
		logger.Error(err, "Unable to update node metadata")

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

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		logger.Error(err, "Failed to get remote client for workload cluster", "cluster key", util.ObjectKey(cluster))

		return ctrl.Result{}, fmt.Errorf("getting workload cluster: %w", err)
	}

	if err := workloadCluster.InitWorkload(ctx, controlPlane); err != nil {
		return ctrl.Result{}, err
	}

	switch rcp.Spec.RolloutStrategy.Type {
	case controlplanev1.RollingUpdateStrategyType:
		// RolloutStrategy is currently defaulted and validated to be RollingUpdate.
		// Defaulted to 1 if not specified
		maxSurge := intstr.FromInt(1)
		if rcp.Spec.RolloutStrategy.RollingUpdate != nil && rcp.Spec.RolloutStrategy.RollingUpdate.MaxSurge != nil {
			maxSurge = *rcp.Spec.RolloutStrategy.RollingUpdate.MaxSurge
		}

		maxNodes := *rcp.Spec.Replicas + rke2util.SafeInt32(maxSurge.IntValue())
		if rke2util.SafeInt32(controlPlane.Machines.Len()) < maxNodes {
			// scaleUpControlPlane ensures that we don't continue scaling up while waiting for Machines to have NodeRefs
			return r.scaleUpControlPlane(ctx, cluster, rcp, controlPlane)
		}

		return r.scaleDownControlPlane(ctx, cluster, rcp, controlPlane, machinesRequireUpgrade)
	default:
		err := fmt.Errorf("unknown rollout strategy type %q", rcp.Spec.RolloutStrategy.Type)
		logger.Error(err, "RolloutStrategy type is not set to RollingUpdateStrategyType, unable to determine the strategy for rolling out machines")

		return ctrl.Result{}, nil
	}
}

func (r *RKE2ControlPlaneReconciler) reconcilePreTerminateHook(ctx context.Context, controlPlane *rke2.ControlPlane) (ctrl.Result, error) {
	// Ensure that every active machine has the drain hook set
	patchHookAnnotation := false

	for _, machine := range controlPlane.Machines.Filter(collections.ActiveMachines) {
		if _, exists := machine.Annotations[controlplanev1.PreTerminateHookCleanupAnnotation]; !exists {
			machine.Annotations[controlplanev1.PreTerminateHookCleanupAnnotation] = ""
			patchHookAnnotation = true
		}
	}

	if patchHookAnnotation {
		// Patch machine annoations
		if err := controlPlane.PatchMachines(ctx); err != nil {
			return ctrl.Result{}, err
		}
	}

	if !controlPlane.HasDeletingMachine() {
		return ctrl.Result{}, nil
	}

	log := ctrl.LoggerFrom(ctx)

	// Return early, if there is already a deleting Machine without the pre-terminate hook.
	// We are going to wait until this Machine goes away before running the pre-terminate hook on other Machines.
	for _, deletingMachine := range controlPlane.DeletingMachines() {
		if _, exists := deletingMachine.Annotations[controlplanev1.PreTerminateHookCleanupAnnotation]; !exists {
			return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
		}
	}

	// Pick the Machine with the oldest deletionTimestamp to keep this function deterministic / reentrant
	// so we only remove the pre-terminate hook from one Machine at a time.
	deletingMachines := controlPlane.DeletingMachines()
	deletingMachine := controlPlane.SortedByDeletionTimestamp(deletingMachines)[0]

	log = log.WithValues("Machine", klog.KObj(deletingMachine))
	ctx = ctrl.LoggerInto(ctx, log)

	// Return early if there are other pre-terminate hooks for the Machine.
	// The CAPRKE2 pre-terminate hook should be the one executed last, so that kubelet
	// is still working while other pre-terminate hooks are run.
	if machineHasOtherPreTerminateHooks(deletingMachine) {
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// Return early because the Machine controller is not yet waiting for the pre-terminate hook.
	c := conditions.Get(deletingMachine, clusterv1.PreTerminateDeleteHookSucceededCondition)
	if c == nil || c.Status != corev1.ConditionFalse || c.Reason != clusterv1.WaitingExternalHookReason {
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// The following will execute and remove the pre-terminate hook from the Machine.

	// Skip leader change for legacy CP
	_, found := controlPlane.RCP.Annotations[controlplanev1.LegacyRKE2ControlPlane]

	// If we have more than 1 Machine and etcd is managed we forward etcd leadership and remove the member
	// to keep the etcd cluster healthy.
	if controlPlane.Machines.Len() > 1 && !found {
		workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err,
				"failed to remove etcd member for deleting Machine %s: failed to create client to workload cluster", klog.KObj(deletingMachine))
		}

		// Note: In regular deletion cases (remediation, scale down) the leader should have been already moved.
		// We're doing this again here in case the Machine became leader again or the Machine deletion was
		// triggered in another way (e.g. a user running kubectl delete machine)
		etcdLeaderCandidate := controlPlane.Machines.Filter(collections.Not(collections.HasDeletionTimestamp)).Newest()
		if etcdLeaderCandidate != nil {
			if err := workloadCluster.ForwardEtcdLeadership(ctx, deletingMachine, etcdLeaderCandidate); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to move leadership to candidate Machine %s", etcdLeaderCandidate.Name)
			}
		} else {
			log.Info("Skip forwarding etcd leadership, because there is no other control plane Machine without a deletionTimestamp")
		}

		// Note: Removing the etcd member will lead to the etcd and the kube-apiserver Pod on the Machine shutting down.
		if err := workloadCluster.RemoveEtcdMemberForMachine(ctx, deletingMachine); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to remove etcd member for deleting Machine %s", klog.KObj(deletingMachine))
		}
	}

	if err := r.removePreTerminateHookAnnotationFromMachine(ctx, deletingMachine); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Waiting for Machines to be deleted", "machines",
		strings.Join(controlPlane.Machines.Filter(collections.HasDeletionTimestamp).Names(), ", "))

	return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
}

func machineHasOtherPreTerminateHooks(machine *clusterv1.Machine) bool {
	for k := range machine.Annotations {
		if strings.HasPrefix(k, clusterv1.PreTerminateDeleteHookAnnotationPrefix) && k != controlplanev1.PreTerminateHookCleanupAnnotation {
			return true
		}
	}

	return false
}

// syncMachines updates Machines, InfrastructureMachines and Rke2Configs to propagate in-place mutable fields from RKE2ControlPlane.
// Note: For InfrastructureMachines and Rke2Configs it also drops ownership of "metadata.labels" and
// "metadata.annotations" from "manager" so that "rke2controlplane" can own these fields and can work with SSA.
// Otherwise, fields would be co-owned by our "old" "manager" and "rke2controlplane" and then we would not be
// able to e.g. drop labels and annotations.
func (r *RKE2ControlPlaneReconciler) syncMachines(ctx context.Context, controlPlane *rke2.ControlPlane) error {
	patchHelpers := map[string]*patch.Helper{}

	for machineName := range controlPlane.Machines {
		m := controlPlane.Machines[machineName]
		// If the Machine is already being deleted, we only need to sync
		// the subset of fields that impact tearing down the Machine.
		if !m.DeletionTimestamp.IsZero() {
			patchHelper, err := patch.NewHelper(m, r.Client)
			if err != nil {
				return err
			}

			// Set all other in-place mutable fields that impact the ability to tear down existing machines.
			m.Spec.NodeDrainTimeout = controlPlane.RCP.Spec.MachineTemplate.NodeDrainTimeout
			m.Spec.NodeDeletionTimeout = controlPlane.RCP.Spec.MachineTemplate.NodeDeletionTimeout
			m.Spec.NodeVolumeDetachTimeout = controlPlane.RCP.Spec.MachineTemplate.NodeVolumeDetachTimeout

			if err := patchHelper.Patch(ctx, m); err != nil {
				return err
			}

			controlPlane.Machines[machineName] = m
			patchHelper, err = patch.NewHelper(m, r.Client)
			if err != nil { //nolint:wsl
				return err
			}

			patchHelpers[machineName] = patchHelper

			continue
		}

		// Cleanup managed fields of all Machines.
		if err := ssa.CleanUpManagedFieldsForSSAAdoption(ctx, r.Client, m, rke2ManagerName); err != nil {
			return errors.Wrapf(err, "failed to update Machine: failed to adjust the managedFields of the Machine %s", klog.KObj(m))
		}
		// Update Machine to propagate in-place mutable fields from RCP.
		updatedMachine, err := r.UpdateMachine(ctx, m, controlPlane.RCP, controlPlane.Cluster)
		if err != nil {
			return errors.Wrapf(err, "failed to update Machine: %s", klog.KObj(m))
		}

		controlPlane.Machines[machineName] = updatedMachine
		// Since the machine is updated, re-create the patch helper so that any subsequent
		// Patch calls use the correct base machine object to calculate the diffs.
		// Example: reconcileControlPlaneConditions patches the machine objects in a subsequent call
		// and, it should use the updated machine to calculate the diff.
		// Note: If the patchHelpers are not re-computed based on the new updated machines, subsequent
		// Patch calls will fail because the patch will be calculated based on an outdated machine and will error
		// because of outdated resourceVersion.
		patchHelper, err := patch.NewHelper(updatedMachine, r.Client)
		if err != nil {
			return errors.Wrapf(err, "failed to create patch helper for Machine %s", klog.KObj(updatedMachine))
		}

		patchHelpers[machineName] = patchHelper

		labelsAndAnnotationsManagedFieldPaths := []contract.Path{
			{"f:metadata", "f:annotations"},
			{"f:metadata", "f:labels"},
		}
		infraMachine, infraMachineFound := controlPlane.InfraResources[machineName]
		// Only update the InfraMachine if it is already found, otherwise just skip it.
		// This could happen e.g. if the cache is not up-to-date yet.
		if infraMachineFound {
			// Cleanup managed fields of all InfrastructureMachines to drop ownership of labels and annotations
			// from "manager". We do this so that InfrastructureMachines that are created using the Create method
			// can also work with SSA. Otherwise, labels and annotations would be co-owned by our "old" "manager"
			// and "rke2-controlplane" and then we would not be able to e.g. drop labels and annotations.
			if err := ssa.DropManagedFields(ctx, r.Client, infraMachine, rke2ManagerName, labelsAndAnnotationsManagedFieldPaths); err != nil {
				return errors.Wrapf(err, "failed to clean up managedFields of InfrastructureMachine %s", klog.KObj(infraMachine))
			}
			// Update in-place mutating fields on InfrastructureMachine.
			if err := r.UpdateExternalObject(ctx, infraMachine, controlPlane.RCP, controlPlane.Cluster); err != nil {
				return errors.Wrapf(err, "failed to update InfrastructureMachine %s", klog.KObj(infraMachine))
			}
		}

		rke2Config, rke2ConfigFound := controlPlane.Rke2Configs[machineName]
		// Only update the RKE2Config if it is already found, otherwise just skip it.
		// This could happen e.g. if the cache is not up-to-date yet.
		if rke2ConfigFound {
			// Note: Set the GroupVersionKind because updateExternalObject depends on it.
			rke2Config.SetGroupVersionKind(m.Spec.Bootstrap.ConfigRef.GroupVersionKind())
			// Cleanup managed fields of all RKE2Configs to drop ownership of labels and annotations
			// from "manager". We do this so that RKE2Configs that are created using the Create method
			// can also work with SSA. Otherwise, labels and annotations would be co-owned by our "old" "manager"
			// and "rke2-controlplane" and then we would not be able to e.g. drop labels and annotations.
			if err := ssa.DropManagedFields(ctx, r.Client, rke2Config, rke2ManagerName, labelsAndAnnotationsManagedFieldPaths); err != nil {
				return errors.Wrapf(err, "failed to clean up managedFields of RKE2Config %s", klog.KObj(rke2Config))
			}
			// Update in-place mutating fields on BootstrapConfig.
			if err := r.UpdateExternalObject(ctx, rke2Config, controlPlane.RCP, controlPlane.Cluster); err != nil {
				return errors.Wrapf(err, "failed to update RKE2Config %s", klog.KObj(rke2Config))
			}
		}
	}
	// Update the patch helpers.
	controlPlane.SetPatchHelpers(patchHelpers)

	return nil
}
