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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1alpha1"
	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1alpha1"
	bsutil "github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"

	//clog "sigs.k8s.io/cluster-api/util/log"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Rke2ConfigReconciler reconciles a Rke2Config object
type Rke2ConfigReconciler struct {
	RKE2InitLock RKE2InitLock
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=rke2configs;rke2configs/status;rke2configs/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;machinesets;machines;machines/status;machinepools;machinepools/status,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets;events;configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Rke2Config object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *Rke2ConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rerr error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconcile RKE2Config")

	config := &bootstrapv1.RKE2Config{}

	if err := r.Get(ctx, req.NamespacedName, config, &client.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Error(err, "rke2Config not found", "rke2-config-name", req.NamespacedName)
			return ctrl.Result{}, err
		}
		logger.Error(err, "", "rke2-config-namespaced-name", req.NamespacedName)
		return ctrl.Result{Requeue: true}, err
	}

	scope := &Scope{}

	// ctx, logger, err := clog.AddOw
	cp, err := bsutil.GetOwnerControlPlane(ctx, r.Client, config.ObjectMeta)
	if err != nil {
		logger.Error(err, "Failed to retrieve owner ControlPlane from the API Server", config.Namespace+"/"+config.Name, "cluster", cp.Name)
		return ctrl.Result{}, err
	}
	if cp == nil {
		logger.Info("This config is for a workerNode")
		scope.HasControlPlaneOwner = false
	} else {
		logger.Info("This config is for a ControlPlane node")
		scope.HasControlPlaneOwner = true
		scope.ControlPlane = cp
		logger = logger.WithValues(cp.Kind, cp.GetNamespace()+"/"+cp.GetName(), "resourceVersion", cp.GetResourceVersion())
	}

	machine, err := util.GetOwnerMachine(ctx, r.Client, config.ObjectMeta)
	if err != nil {
		logger.Error(err, "Failed to retrieve owner Machine from the API Server", config.Namespace+"/"+config.Name, "machine", machine.Name)
		return ctrl.Result{}, err
	}
	if machine == nil {
		logger.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{Requeue: true}, nil
	}
	scope.Machine = machine
	logger = logger.WithValues(machine.Kind, machine.GetNamespace()+"/"+machine.GetName(), "resourceVersion", machine.GetResourceVersion())

	cluster, err := util.GetClusterByName(ctx, r.Client, machine.GetNamespace(), machine.Spec.ClusterName)
	if err != nil {
		if errors.Cause(err) == util.ErrNoCluster {
			logger.Info(fmt.Sprintf("%s does not belong to a cluster yet, waiting until it's part of a cluster", machine.Kind))
			return ctrl.Result{}, nil
		}

		if apierrors.IsNotFound(err) {
			logger.Info("Cluster does not exist yet, waiting until it is created")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Could not get cluster with metadata")
		return ctrl.Result{}, err
	}

	if annotations.IsPaused(cluster, config) {
		logger.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	scope.Cluster = cluster

	scope.Logger = logger
	ctx = ctrl.LoggerInto(ctx, logger)

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(config, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Attempt to Patch the KubeadmConfig object and status after each reconciliation if no error occurs.
	defer func() {
		// always update the readyCondition; the summary is represented using the "1 of x completed" notation.
		conditions.SetSummary(config,
			conditions.WithConditions(
				bootstrapv1.DataSecretAvailableCondition,
			),
		)
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{}
		if rerr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchHelper.Patch(ctx, config, patchOpts...); err != nil {
			logger.Error(rerr, "Failed to patch config")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	if !machine.Status.InfrastructureReady {
		logger.Info("Infrastructure machine not yet ready")
		conditions.MarkFalse(config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{Requeue: true}, nil
	}
	// Migrate plaintext data to secret.
	// if config.Status.BootstrapData != nil && config.Status.DataSecretName == nil {
	// 	return ctrl.Result{}, r.storeBootstrapData(ctx, scope, config.Status.BootstrapData)
	// }
	// Reconcile status for machines that already have a secret reference, but our status isn't up to date.
	// This case solves the pivoting scenario (or a backup restore) which doesn't preserve the status subresource on objects.
	if machine.Spec.Bootstrap.DataSecretName != nil && (!config.Status.Ready || config.Status.DataSecretName == nil) {
		config.Status.Ready = true
		config.Status.DataSecretName = machine.Spec.Bootstrap.DataSecretName
		conditions.MarkTrue(config, bootstrapv1.DataSecretAvailableCondition)
		return ctrl.Result{}, nil
	}
	// Status is ready means a config has been generated.
	if config.Status.Ready {
		// In any other case just return as the config is already generated and need not be generated again.
		conditions.MarkTrue(config, bootstrapv1.DataSecretAvailableCondition)
		return ctrl.Result{}, nil
	}

	// Note: can't use IsFalse here because we need to handle the absence of the condition as well as false.
	if !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		return r.handleClusterNotInitialized(ctx, scope)
	}

	// Every other case it's a join scenario
	// Nb. in this case ClusterConfiguration and InitConfiguration should not be defined by users, but in case of misconfigurations, CABPK simply ignore them

	// Unlock any locks that might have been set during init process
	r.RKE2InitLock.Unlock(ctx, cluster)

	// it's a control plane join
	if scope.HasControlPlaneOwner {
		return r.joinControlplane(ctx, scope)
	}

	// It's a worker join
	return r.joinWorker(ctx, scope)
	// if machine.Status.Phase != string(clusterv1.MachinePhasePending) {
	// 	logger.Info("Machine is not in pending state")
	// 	return ctrl.Result{Requeue: true}, nil
	// }

	// return ctrl.Result{}, nil
}

// Scope is a scoped struct used during reconciliation.
type Scope struct {
	logr.Logger
	Config               *bootstrapv1.RKE2Config
	Machine              *clusterv1.Machine
	Cluster              *clusterv1.Cluster
	HasControlPlaneOwner bool
	ControlPlane         *controlplanev1.RKE2ControlPlane
}

// SetupWithManager sets up the controller with the Manager.
func (r *Rke2ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bootstrapv1.RKE2Config{}).
		Complete(r)
}

// TODO: Implement these functions

// handleClusterNotInitialized handles the first control plane node
func (r *Rke2ConfigReconciler) handleClusterNotInitialized(ctx context.Context, scope *Scope) (res ctrl.Result, rerr error) {
	return ctrl.Result{}, nil
}

func Unlock(ctx context.Context, cluster *clusterv1.Cluster) bool {
	return true
}

type RKE2InitLock interface {
	Unlock(context.Context, *clusterv1.Cluster) bool
	Lock(context.Context, *clusterv1.Cluster) bool
}

// TODO: Implement these functions
func (r *Rke2ConfigReconciler) joinControlplane(ctx context.Context, scope *Scope) (res ctrl.Result, rerr error) {
	return ctrl.Result{}, nil
}

// TODO: Implement these functions
func (r *Rke2ConfigReconciler) joinWorker(ctx context.Context, scope *Scope) (res ctrl.Result, rerr error) {
	return ctrl.Result{}, nil
}
