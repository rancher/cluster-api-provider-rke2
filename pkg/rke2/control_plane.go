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

package rke2

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	capifd "sigs.k8s.io/cluster-api/util/failuredomains"
	"sigs.k8s.io/cluster-api/util/patch"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
)

// ControlPlane holds business logic around control planes.
// It should never need to connect to a service, that responsibility lies outside of this struct.
// Going forward we should be trying to add more logic to here and reduce the amount of logic in the reconciler.
type ControlPlane struct {
	RCP                  *controlplanev1.RKE2ControlPlane
	Cluster              *clusterv1.Cluster
	Machines             collections.Machines
	machinesPatchHelpers map[string]*patch.Helper

	// reconciliationTime is the time of the current reconciliation, and should be used for all "now" calculations
	reconciliationTime metav1.Time

	Rke2Configs    map[string]*bootstrapv1.RKE2Config
	InfraResources map[string]*unstructured.Unstructured

	managementCluster ManagementCluster
	workloadCluster   WorkloadCluster
}

// NewControlPlane returns an instantiated ControlPlane.
func NewControlPlane(
	ctx context.Context,
	managementCluster ManagementCluster,
	client client.Client,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
	ownedMachines collections.Machines,
) (*ControlPlane, error) {
	infraObjects, err := GetInfraResources(ctx, client, ownedMachines)
	if err != nil {
		return nil, err
	}

	rke2Configs, err := GetRKE2Configs(ctx, client, ownedMachines)
	if err != nil {
		return nil, err
	}

	patchHelpers := map[string]*patch.Helper{}

	for name, machine := range ownedMachines {
		patchHelper, err := patch.NewHelper(machine, client)
		if err != nil {
			if machine.Status.NodeRef != nil {
				_ = machine.Status.NodeRef.Name
			}

			return nil, err
		}

		patchHelpers[name] = patchHelper
	}

	return &ControlPlane{
		RCP:                  rcp,
		Cluster:              cluster,
		Machines:             ownedMachines,
		machinesPatchHelpers: patchHelpers,
		Rke2Configs:          rke2Configs,
		InfraResources:       infraObjects,
		reconciliationTime:   metav1.Now(),
		managementCluster:    managementCluster,
	}, nil
}

// Logger returns a logger with useful context.
func (c *ControlPlane) Logger() logr.Logger {
	return klog.Background().WithValues("namespace", c.RCP.Namespace, "name", c.RCP.Name, "cluster-name", c.Cluster.Name)
}

// FailureDomains returns a slice of failure domain objects synced from the infrastructure provider into Cluster.Status.
func (c *ControlPlane) FailureDomains() clusterv1.FailureDomains {
	if c.Cluster.Status.FailureDomains == nil {
		return clusterv1.FailureDomains{}
	}

	return c.Cluster.Status.FailureDomains
}

// Version returns the RKE2ControlPlane's version.
func (c *ControlPlane) Version() *string {
	version := c.RCP.GetDesiredVersion()

	return &version
}

// InfrastructureRef returns the RKE2ControlPlane's infrastructure template.
func (c *ControlPlane) InfrastructureRef() *corev1.ObjectReference {
	return &c.RCP.Spec.MachineTemplate.InfrastructureRef
}

// AsOwnerReference returns an owner reference to the RKE2ControlPlane.
func (c *ControlPlane) AsOwnerReference() *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "RKE2ControlPlane",
		Name:       c.RCP.Name,
		UID:        c.RCP.UID,
	}
}

// MachineInFailureDomainWithMostMachines returns the first matching failure domain with machines that has the most control-plane machines on it.
func (c *ControlPlane) MachineInFailureDomainWithMostMachines(ctx context.Context, machines collections.Machines) (*clusterv1.Machine, error) {
	fd := c.FailureDomainWithMostMachines(ctx, machines)
	machinesInFailureDomain := machines.Filter(collections.InFailureDomains(fd))
	machineToMark := machinesInFailureDomain.Oldest()

	if machineToMark == nil {
		return nil, errors.New("failed to pick control plane Machine to mark for deletion")
	}

	return machineToMark, nil
}

// MachineWithDeleteAnnotation returns a machine that has been annotated with DeleteMachineAnnotation key.
func (c *ControlPlane) MachineWithDeleteAnnotation(machines collections.Machines) collections.Machines {
	// See if there are any machines with DeleteMachineAnnotation key.
	annotatedMachines := machines.Filter(collections.HasAnnotationKey(clusterv1.DeleteMachineAnnotation))
	// If there are, return list of annotated machines.
	return annotatedMachines
}

// FailureDomainWithMostMachines returns a fd which exists both in machines and control-plane machines and has the most
// control-plane machines on it.
func (c *ControlPlane) FailureDomainWithMostMachines(ctx context.Context, machines collections.Machines) *string {
	// See if there are any Machines that are not in currently defined failure domains first.
	notInFailureDomains := machines.Filter(
		collections.Not(collections.InFailureDomains(c.FailureDomains().FilterControlPlane().GetIDs()...)),
	)
	if len(notInFailureDomains) > 0 {
		// return the failure domain for the oldest Machine not in the current list of failure domains
		// this could be either nil (no failure domain defined) or a failure domain that is no longer defined
		// in the cluster status.
		return notInFailureDomains.Oldest().Spec.FailureDomain
	}

	return capifd.PickMost(ctx, c.Cluster.Status.FailureDomains.FilterControlPlane(), c.Machines, machines)
}

// NextFailureDomainForScaleUp returns the failure domain with the fewest number of up-to-date machines.
func (c *ControlPlane) NextFailureDomainForScaleUp(ctx context.Context) *string {
	if len(c.Cluster.Status.FailureDomains.FilterControlPlane()) == 0 {
		return nil
	}

	return capifd.PickFewest(ctx, c.FailureDomains().FilterControlPlane(), c.Machines, c.UpToDateMachines(ctx))
}

// InitialControlPlaneConfig returns a new RKE2ConfigSpec that is to be used for an initializing control plane.
func (c *ControlPlane) InitialControlPlaneConfig() *bootstrapv1.RKE2ConfigSpec {
	bootstrapSpec := c.RCP.Spec.RKE2ConfigSpec.DeepCopy()

	return bootstrapSpec
}

// JoinControlPlaneConfig returns a new RKE2ConfigSpec that is to be used for joining control planes.
func (c *ControlPlane) JoinControlPlaneConfig() *bootstrapv1.RKE2ConfigSpec {
	bootstrapSpec := c.RCP.Spec.RKE2ConfigSpec.DeepCopy()

	return bootstrapSpec
}

// GenerateRKE2Config generates a new RKE2 config for creating new control plane nodes.
func (c *ControlPlane) GenerateRKE2Config(spec *bootstrapv1.RKE2ConfigSpec) *bootstrapv1.RKE2Config {
	// Create an owner reference without a controller reference because the owning controller is the machine controller
	owner := metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "RKE2ControlPlane",
		Name:       c.RCP.Name,
		UID:        c.RCP.UID,
	}

	bootstrapConfig := &bootstrapv1.RKE2Config{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SimpleNameGenerator.GenerateName(c.RCP.Name + "-"),
			Namespace: c.RCP.Namespace,
			Labels:    ControlPlaneLabelsForCluster(c.Cluster.Name),
			OwnerReferences: []metav1.OwnerReference{
				owner,
			},
		},
		Spec: *spec,
	}

	return bootstrapConfig
}

// ControlPlaneLabelsForCluster returns a set of labels to add to a control plane machine for this specific cluster.
func ControlPlaneLabelsForCluster(clusterName string) map[string]string {
	return map[string]string{
		clusterv1.ClusterNameLabel:         clusterName,
		clusterv1.MachineControlPlaneLabel: "",
	}
}

// NewMachine returns a machine configured to be a part of the control plane.
func (c *ControlPlane) NewMachine(infraRef, bootstrapRef *corev1.ObjectReference, failureDomain *string) *clusterv1.Machine {
	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SimpleNameGenerator.GenerateName(c.RCP.Name + "-"),
			Namespace: c.RCP.Namespace,
			Labels:    ControlPlaneLabelsForCluster(c.Cluster.Name),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(c.RCP, controlplanev1.GroupVersion.WithKind("RKE2ControlPlane")),
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       c.Cluster.Name,
			Version:           c.Version(),
			InfrastructureRef: *infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
			FailureDomain:           failureDomain,
			NodeDrainTimeout:        c.RCP.Spec.MachineTemplate.NodeDrainTimeout,
			NodeVolumeDetachTimeout: c.RCP.Spec.MachineTemplate.NodeVolumeDetachTimeout,
			NodeDeletionTimeout:     c.RCP.Spec.MachineTemplate.NodeDeletionTimeout,
		},
	}
}

// NeedsReplacementNode determines if the control plane needs to create a replacement node during upgrade.
func (c *ControlPlane) NeedsReplacementNode() bool {
	// Can't do anything with an unknown number of desired replicas.
	if c.RCP.Spec.Replicas == nil {
		return false
	}
	// if the number of existing machines is exactly 1 > than the number of replicas.
	return len(c.Machines)+1 == int(*c.RCP.Spec.Replicas)
}

// HasDeletingMachine returns true if any machine in the control plane is in the process of being deleted.
func (c *ControlPlane) HasDeletingMachine() bool {
	return len(c.Machines.Filter(collections.HasDeletionTimestamp)) > 0
}

// DeletingMachines returns machines in the control plane that are in the process of being deleted.
func (c *ControlPlane) DeletingMachines() collections.Machines {
	return c.Machines.Filter(collections.HasDeletionTimestamp)
}

// SortedByDeletionTimestamp returns the machines sorted by deletion timestamp.
func (c *ControlPlane) SortedByDeletionTimestamp(s collections.Machines) []*clusterv1.Machine {
	res := make(machinesByDeletionTimestamp, 0, len(s))
	for _, value := range s {
		res = append(res, value)
	}

	sort.Sort(res)

	return res
}

// MachinesNeedingRollout return a list of machines that need to be rolled out.
func (c *ControlPlane) MachinesNeedingRollout(ctx context.Context) collections.Machines {
	// Ignore machines to be deleted.
	machines := c.Machines.Filter(collections.Not(collections.HasDeletionTimestamp))

	// Return machines if they are scheduled for rollout or if with an outdated configuration.
	return machines.AnyFilter(
		// Machines that do not match with RCP config.
		collections.Not(matchesRCPConfiguration(ctx, c.InfraResources, c.Rke2Configs, c.RCP)),
	)
}

// UpToDateMachines returns the machines that are up-to-date with the control
// plane's configuration and therefore do not require rollout.
func (c *ControlPlane) UpToDateMachines(ctx context.Context) collections.Machines {
	// Ignore machines to be deleted.
	machines := c.Machines.Filter(collections.Not(collections.HasDeletionTimestamp))
	// Filter machines if they are scheduled for rollout or if with an outdated configuration.
	machines.AnyFilter(
		// Machines that do not match with RCP config.
		collections.Not(matchesRCPConfiguration(ctx, c.InfraResources, c.Rke2Configs, c.RCP)),
	)

	return machines.Difference(c.MachinesNeedingRollout(ctx))
}

// GetInfraResources fetches the external infrastructure resource for each machine in the collection
// and returns a map of machine.Name -> infraResource.
func GetInfraResources(ctx context.Context, cl client.Client, machines collections.Machines) (map[string]*unstructured.Unstructured, error) {
	result := map[string]*unstructured.Unstructured{}

	for _, m := range machines {
		infraObj, err := external.Get(ctx, cl, &m.Spec.InfrastructureRef)
		if err != nil {
			if apierrors.IsNotFound(errors.Cause(err)) {
				continue
			}

			return nil, errors.Wrapf(err, "failed to retrieve infra obj for machine %q", m.Name)
		}

		result[m.Name] = infraObj
	}

	return result, nil
}

// GetRKE2Configs fetches the RKE2 config for each machine in the collection and returns a map of machine.Name -> RKE2Config.
func GetRKE2Configs(ctx context.Context, cl client.Client, machines collections.Machines) (map[string]*bootstrapv1.RKE2Config, error) {
	result := map[string]*bootstrapv1.RKE2Config{}

	for name, m := range machines {
		bootstrapRef := m.Spec.Bootstrap.ConfigRef
		if bootstrapRef == nil {
			continue
		}

		machineConfig := &bootstrapv1.RKE2Config{}

		if err := cl.Get(ctx, client.ObjectKey{Name: bootstrapRef.Name, Namespace: m.Namespace}, machineConfig); err != nil {
			if apierrors.IsNotFound(errors.Cause(err)) {
				continue
			}

			if m.Status.NodeRef != nil {
				name = m.Status.NodeRef.Name
			}

			return nil, errors.Wrapf(err, "failed to retrieve bootstrap config for machine %q with node %s", m.Name, name)
		}

		result[name] = machineConfig
	}

	return result, nil
}

// IsEtcdManaged returns true if the control plane relies on a managed etcd.
func (c *ControlPlane) IsEtcdManaged() bool {
	return true
}

// MachinesToBeRemediatedByRCP returns the list of control plane machines to be remediated by RCP.
func (c *ControlPlane) MachinesToBeRemediatedByRCP() collections.Machines {
	return c.Machines.Filter(collections.IsUnhealthyAndOwnerRemediated)
}

// UnhealthyMachines returns the list of control plane machines marked as unhealthy by MHC.
func (c *ControlPlane) UnhealthyMachines() collections.Machines {
	return c.Machines.Filter(collections.IsUnhealthy)
}

// HealthyMachines returns the list of control plane machines not marked as unhealthy by MHC.
func (c *ControlPlane) HealthyMachines() collections.Machines {
	return c.Machines.Filter(collections.Not(collections.IsUnhealthy))
}

// HasUnhealthyMachine returns true if any machine in the control plane is marked as unhealthy by MHC.
func (c *ControlPlane) HasUnhealthyMachine() bool {
	return len(c.UnhealthyMachines()) > 0
}

// PatchMachines patches the machines in the control plane.
func (c *ControlPlane) PatchMachines(ctx context.Context) error {
	errList := []error{}

	for name := range c.Machines {
		machine := c.Machines[name]
		if helper, ok := c.machinesPatchHelpers[name]; ok {
			if err := helper.Patch(ctx, machine, patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				controlplanev1.MachineAgentHealthyCondition,
				controlplanev1.MachineEtcdMemberHealthyCondition,
				controlplanev1.NodeMetadataUpToDate,
			}}); err != nil {
				if machine.Status.NodeRef != nil {
					_ = machine.Status.NodeRef.Name
				}

				errList = append(errList, err)
			}

			continue
		}

		if machine.Status.NodeRef != nil {
			name = machine.Status.NodeRef.Name
		}

		errList = append(errList, errors.Errorf("failed to get patch helper for machine %s with node %s", machine.Name, name))
	}

	return kerrors.NewAggregate(errList)
}

// HasHealthyMachineStillProvisioning returns true if any healthy machine in the control plane is still in the process of being provisioned.
func (c *ControlPlane) HasHealthyMachineStillProvisioning() bool {
	return len(c.HealthyMachines().Filter(collections.Not(collections.HasNode()))) > 0
}

// GetWorkloadCluster builds a cluster object.
// The cluster comes with an etcd client generator to connect to any etcd pod living on a managed machine.
func (c *ControlPlane) GetWorkloadCluster(ctx context.Context) (WorkloadCluster, error) {
	if c.workloadCluster != nil {
		return c.workloadCluster, nil
	}

	workloadCluster, err := c.managementCluster.GetWorkloadCluster(ctx, client.ObjectKeyFromObject(c.Cluster))
	if err != nil {
		return nil, err
	}

	c.workloadCluster = workloadCluster

	return c.workloadCluster, nil
}

// machinesByDeletionTimestamp sorts a list of Machines by deletion timestamp, using their names as a tie breaker.
// Machines without DeletionTimestamp go after machines with this field set.
type machinesByDeletionTimestamp []*clusterv1.Machine

func (o machinesByDeletionTimestamp) Len() int      { return len(o) }
func (o machinesByDeletionTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o machinesByDeletionTimestamp) Less(i, j int) bool {
	if o[i].DeletionTimestamp == nil && o[j].DeletionTimestamp == nil {
		return o[i].Name < o[j].Name
	}

	if o[i].DeletionTimestamp == nil {
		return false
	}

	if o[j].DeletionTimestamp == nil {
		return true
	}

	if o[i].DeletionTimestamp.Equal(o[j].DeletionTimestamp) {
		return o[i].Name < o[j].Name
	}

	return o[i].DeletionTimestamp.Before(o[j].DeletionTimestamp)
}

// SetPatchHelpers updates the patch helpers.
func (c *ControlPlane) SetPatchHelpers(patchHelpers map[string]*patch.Helper) {
	c.machinesPatchHelpers = patchHelpers
}

// ReconcileExternalReference reconciles Cluster ownership on (Infra)MachineTemplates.
func (c *ControlPlane) ReconcileExternalReference(ctx context.Context, cl client.Client) error {
	logger := log.FromContext(ctx)
	ref := &c.RCP.Spec.MachineTemplate.InfrastructureRef

	if !strings.HasSuffix(ref.Kind, clusterv1.TemplateSuffix) {
		return nil
	}

	if err := utilconversion.UpdateReferenceAPIContract(ctx, cl, ref); err != nil {
		return fmt.Errorf("updating reference API contract: %w", err)
	}

	obj, err := external.Get(ctx, cl, ref)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("Could not find external object %s %s/%s.",
				obj.GroupVersionKind().Kind,
				obj.GetNamespace(),
				obj.GetName()))

			return nil
		}

		return fmt.Errorf("getting external object: %w", err)
	}

	// Note: We intentionally do not handle checking for the paused label on an external template reference

	desiredOwnerRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       c.Cluster.Name,
		UID:        c.Cluster.UID,
	}

	if util.HasExactOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef) {
		return nil
	}

	patchHelper, err := patch.NewHelper(obj, cl)
	if err != nil {
		return fmt.Errorf("initializing patch helper: %w", err)
	}

	obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef))

	return patchHelper.Patch(ctx, obj)
}

// UsesEmbeddedEtcd returns true if the control plane does not define an external datastore configuration.
func (c *ControlPlane) UsesEmbeddedEtcd() bool {
	return c.RCP.Spec.ServerConfig.ExternalDatastoreSecret == nil
}
