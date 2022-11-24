package rke2

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"

	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/machinefilters"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ManagementCluster defines all behaviors necessary for something to function as a management cluster.
type ManagementCluster interface {
	ctrlclient.Reader

	GetMachinesForCluster(ctx context.Context, cluster ctrlclient.ObjectKey, filters ...machinefilters.Func) (FilterableMachineCollection, error)
	GetWorkloadCluster(ctx context.Context, clusterKey ctrlclient.ObjectKey) (WorkloadCluster, error)
}

// Management holds operations on the management cluster.
type Management struct {
	Client ctrlclient.Reader
}

// RemoteClusterConnectionError represents a failure to connect to a remote cluster
type RemoteClusterConnectionError struct {
	Name string
	Err  error
}

func (e *RemoteClusterConnectionError) Error() string { return e.Name + ": " + e.Err.Error() }
func (e *RemoteClusterConnectionError) Unwrap() error { return e.Err }

// Get implements ctrlclient.Reader
func (m *Management) Get(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object, opts ...ctrlclient.GetOption) error {
	return m.Client.Get(ctx, key, obj, opts...)
}

// List implements ctrlclient.Reader
func (m *Management) List(ctx context.Context, list ctrlclient.ObjectList, opts ...ctrlclient.ListOption) error {
	return m.Client.List(ctx, list, opts...)
}

// GetMachinesForCluster returns a list of machines that can be filtered or not.
// If no filter is supplied then all machines associated with the target cluster are returned.
func (m *Management) GetMachinesForCluster(ctx context.Context, cluster ctrlclient.ObjectKey, filters ...machinefilters.Func) (FilterableMachineCollection, error) {
	logger := log.FromContext(ctx)
	selector := map[string]string{
		clusterv1.ClusterLabelName: cluster.Name,
	}
	ml := &clusterv1.MachineList{}
	logger.Info("Getting List of machines for Cluster")
	if err := m.Client.List(ctx, ml, ctrlclient.InNamespace(cluster.Namespace), ctrlclient.MatchingLabels(selector)); err != nil {
		return nil, errors.Wrap(err, "failed to list machines")
	}
	logger.Info("End of listing machines for cluster")
	machines := NewFilterableMachineCollectionFromMachineList(ml)
	return machines.Filter(filters...), nil
}

const (
	// RKE2ControlPlaneControllerName defines the controller used when creating clients.
	RKE2ControlPlaneControllerName = "rke2-controlplane-controller"
)

// GetWorkloadCluster builds a cluster object.
// The cluster comes with an etcd client generator to connect to any etcd pod living on a managed machine.
func (m *Management) GetWorkloadCluster(ctx context.Context, clusterKey ctrlclient.ObjectKey) (WorkloadCluster, error) {
	restConfig, err := remote.RESTConfig(ctx, RKE2ControlPlaneControllerName, m.Client, clusterKey)
	if err != nil {
		return nil, err
	}
	restConfig.Timeout = 30 * time.Second

	c, err := ctrlclient.New(restConfig, ctrlclient.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, &RemoteClusterConnectionError{Name: clusterKey.String(), Err: err}
	}

	return &Workload{
		Client: c,
	}, nil
}
