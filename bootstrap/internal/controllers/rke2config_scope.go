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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	exputil "sigs.k8s.io/cluster-api/exp/util"
	"sigs.k8s.io/cluster-api/util"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	bsutil "github.com/rancher/cluster-api-provider-rke2/pkg/util"
)

var (
	// ErrRKE2ConfigNotFound describes the RKE2Config is not found.
	ErrRKE2ConfigNotFound = errors.New("RKE2Config is not found")
	// ErrNoRKE2ConfigOwner describes the RKE2Config has no Machine or MachinePool owner.
	ErrNoRKE2ConfigOwner = errors.New("RKE2Config has no owner")
	// ErrVersionNotFound describes the desired K8S version can not be found.
	ErrVersionNotFound = errors.New("RKE2 version can not be found")
)

// Scope is a scoped struct used during reconciliation.
type Scope struct {
	Logger       logr.Logger
	Config       *bootstrapv1.RKE2Config
	Machine      *clusterv1.Machine
	MachinePool  *clusterv1.MachinePool
	Cluster      *clusterv1.Cluster
	ControlPlane *controlplanev1.RKE2ControlPlane
}

// NewScope initializes the RKE2Config scope given a new request.
func NewScope(ctx context.Context, req ctrl.Request, client client.Client) (*Scope, error) {
	logger := log.FromContext(ctx)
	config := &bootstrapv1.RKE2Config{}

	// Fetch the RKE2Config
	if err := client.Get(ctx, req.NamespacedName, config); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrRKE2ConfigNotFound
		}

		return nil, fmt.Errorf("fetching RKE2Config %s: %w", req.NamespacedName, err)
	}

	logger = logger.WithValues("RKE2Config", req.NamespacedName)

	// Fetch the Machine or MachinePool owner
	clusterName := ""

	machine, err := util.GetOwnerMachine(ctx, client, config.ObjectMeta)
	if err != nil {
		return nil, fmt.Errorf("fetching Machine owner: %w", err)
	}

	if machine != nil {
		logger = logger.WithValues("MachineOwner", machine.Name)
		clusterName = machine.Spec.ClusterName
	}

	machinePool, err := exputil.GetOwnerMachinePool(ctx, client, config.ObjectMeta)
	if err != nil {
		return nil, fmt.Errorf("fetching MachinePool owner: %w", err)
	}

	if machinePool != nil {
		logger = logger.WithValues("MachinePoolOwner", machinePool.Name)
		clusterName = machinePool.Spec.ClusterName
	}

	if machine == nil && machinePool == nil {
		return nil, ErrNoRKE2ConfigOwner
	}

	// Fetch the ControlPlane associated to the Machine owner if any
	var cp *controlplanev1.RKE2ControlPlane

	if machine != nil {
		cp, err = bsutil.GetOwnerControlPlane(ctx, client, machine.ObjectMeta)
		if err != nil && !errors.Is(err, bsutil.ErrControlPlaneNotFound) {
			return nil, fmt.Errorf("fetching ControlPlane owner: %w", err)
		}
	}

	if cp != nil {
		logger = logger.WithValues("ControlPlaneOwner", cp.Name)
	}

	// Fetch the Cluster
	cluster, err := util.GetClusterByName(ctx, client, config.Namespace, clusterName)
	if err != nil {
		return nil, fmt.Errorf("fetching Cluster: %w", err)
	}

	return &Scope{
		Config:       config,
		Machine:      machine,
		MachinePool:  machinePool,
		Logger:       logger,
		ControlPlane: cp,
		Cluster:      cluster,
	}, nil
}

// HasMachineOwner returns true if the RKE2Config is owned by a Machine.
func (s *Scope) HasMachineOwner() bool {
	return s.Machine != nil
}

// HasMachinePoolOwner returns true if the RKE2Config is owned by a MachinePool.
func (s *Scope) HasMachinePoolOwner() bool {
	return s.MachinePool != nil
}

// HasControlPlaneOwner returns true if the RKE2Config is owned by a Machine which is also a ControlPlane.
func (s *Scope) HasControlPlaneOwner() bool {
	return s.Machine != nil && s.ControlPlane != nil
}

// GetDesiredVersion returns the K8S version associated to the RKE2Config owner.
func (s *Scope) GetDesiredVersion() (string, error) {
	if s.MachinePool != nil && s.MachinePool.Spec.Template.Spec.Version != "" {
		return s.MachinePool.Spec.Template.Spec.Version, nil
	}

	if s.Machine != nil && s.Machine.Spec.Version != "" {
		return s.Machine.Spec.Version, nil
	}

	return "", ErrVersionNotFound
}
