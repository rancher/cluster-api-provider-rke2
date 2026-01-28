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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	capifd "sigs.k8s.io/cluster-api/util/failuredomains"
	"sigs.k8s.io/cluster-api/util/patch"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
)

// ControlPlane holds business logic around control planes.
// It should never need to connect to a service, that responsibility lies outside of this struct.
// Going forward we should be trying to add more logic to here and reduce the amount of logic in the reconciler.
type ControlPlane struct {
	RCP                  *controlplanev1.RKE2ControlPlane
	Cluster              *clusterv1.Cluster
	Machines             collections.Machines
	machinesPatchHelpers map[string]*patch.Helper

	machinesNotUptoDate                  collections.Machines
	machinesNotUptoDateLogMessages       map[string][]string
	machinesNotUptoDateConditionMessages map[string][]string

	// InfraMachineTemplateIsNotFound is true if getting the infra machine template object failed with an NotFound err
	InfraMachineTemplateIsNotFound bool

	// PreflightChecks contains description about pre flight check results blocking machines creation or deletion.
	PreflightCheckResults PreflightCheckResults

	Rke2Configs    map[string]*bootstrapv1.RKE2Config
	InfraResources map[string]*unstructured.Unstructured

	managementCluster ManagementCluster
	workloadCluster   WorkloadCluster

	// deletingReason is the reason that should be used when setting the Deleting condition.
	DeletingReason string

	// deletingMessage is the message that should be used when setting the Deleting condition.
	DeletingMessage string
}

// PreflightCheckResults contains description about pre flight check results blocking machines creation or deletion.
type PreflightCheckResults struct {
	// HasDeletingMachine reports true if preflight check detected a deleting machine.
	HasDeletingMachine bool
	// ControlPlaneComponentsNotHealthy reports true if preflight check detected that the control plane components are not fully healthy.
	ControlPlaneComponentsNotHealthy bool
	// EtcdClusterNotHealthy reports true if preflight check detected that the etcd cluster is not fully healthy.
	EtcdClusterNotHealthy bool
	// TopologyVersionMismatch reports true if preflight check detected that the Cluster's topology version does not match the control plane's version
	TopologyVersionMismatch bool
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
			if machine.Status.NodeRef.IsDefined() {
				_ = machine.Status.NodeRef.Name
			}

			return nil, err
		}

		patchHelpers[name] = patchHelper
	}

	machinesNotUptoDate := make(collections.Machines, len(ownedMachines))
	machinesNotUptoDateLogMessages := map[string][]string{}
	machinesNotUptoDateConditionMessages := map[string][]string{}

	for _, m := range ownedMachines {
		upToDate, logMessages, conditionMessages, err := UpToDate(ctx, m, rcp, infraObjects, rke2Configs)
		if err != nil {
			return nil, err
		}

		if !upToDate {
			machinesNotUptoDate.Insert(m)

			machinesNotUptoDateLogMessages[m.Name] = logMessages
			machinesNotUptoDateConditionMessages[m.Name] = conditionMessages
		}
	}

	return &ControlPlane{
		RCP:                                  rcp,
		Cluster:                              cluster,
		Machines:                             ownedMachines,
		machinesPatchHelpers:                 patchHelpers,
		machinesNotUptoDate:                  machinesNotUptoDate,
		machinesNotUptoDateLogMessages:       machinesNotUptoDateLogMessages,
		machinesNotUptoDateConditionMessages: machinesNotUptoDateConditionMessages,
		Rke2Configs:                          rke2Configs,
		InfraResources:                       infraObjects,
		managementCluster:                    managementCluster,
	}, nil
}

// Logger returns a logger with useful context.
func (c *ControlPlane) Logger() logr.Logger {
	return klog.Background().WithValues("namespace", c.RCP.Namespace, "name", c.RCP.Name, "cluster-name", c.Cluster.Name)
}

// FailureDomains returns a slice of failure domain objects synced from the infrastructure provider into Cluster.Status.
func (c *ControlPlane) FailureDomains() []clusterv1.FailureDomain {
	if c.Cluster.Status.FailureDomains == nil {
		return nil
	}

	var res []clusterv1.FailureDomain

	for _, spec := range c.Cluster.Status.FailureDomains {
		if ptr.Deref(spec.ControlPlane, false) {
			res = append(res, spec)
		}
	}

	return res
}

// Version returns the RKE2ControlPlane's version.
func (c *ControlPlane) Version() string {
	version := c.RCP.GetDesiredVersion()

	return version
}

// InfrastructureRef returns the RKE2ControlPlane's infrastructure template.
func (c *ControlPlane) InfrastructureRef() clusterv1.ContractVersionedObjectReference {
	return c.RCP.Spec.MachineTemplate.Spec.InfrastructureRef
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
func (c *ControlPlane) FailureDomainWithMostMachines(ctx context.Context, machines collections.Machines) string {
	// See if there are any Machines that are not in currently defined failure domains first.
	notInFailureDomains := machines.Filter(
		collections.Not(collections.InFailureDomains(getFailureDomainIDs(c.FailureDomains())...)),
	)
	if len(notInFailureDomains) > 0 {
		// return the failure domain for the oldest Machine not in the current list of failure domains
		// this could be either nil (no failure domain defined) or a failure domain that is no longer defined
		// in the cluster status.
		return notInFailureDomains.Oldest().Spec.FailureDomain
	}

	return capifd.PickMost(ctx, c.FailureDomains(), c.Machines, machines)
}

// NextFailureDomainForScaleUp returns the failure domain with the fewest number of up-to-date machines.
func (c *ControlPlane) NextFailureDomainForScaleUp(ctx context.Context) (string, error) {
	if len(c.FailureDomains()) == 0 {
		return "", nil
	}

	return capifd.PickFewest(ctx, c.FailureDomains(), c.Machines, c.UpToDateMachines().Filter(collections.Not(collections.HasDeletionTimestamp))), nil
}

func getFailureDomainIDs(failureDomains []clusterv1.FailureDomain) []string {
	ids := make([]string, 0, len(failureDomains))
	for _, fd := range failureDomains {
		ids = append(ids, fd.Name)
	}

	return ids
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
func (c *ControlPlane) NewMachine(infraRef, bootstrapRef clusterv1.ContractVersionedObjectReference, failureDomain string) *clusterv1.Machine {
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
			InfrastructureRef: infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
			FailureDomain: failureDomain,
			Deletion: clusterv1.MachineDeletionSpec{
				NodeDrainTimeoutSeconds:        c.RCP.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds,
				NodeVolumeDetachTimeoutSeconds: c.RCP.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds,
				NodeDeletionTimeoutSeconds:     c.RCP.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds,
			},
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
func (c *ControlPlane) UpToDateMachines() collections.Machines {
	return c.Machines.Difference(c.machinesNotUptoDate)
}

// NotUpToDateMachines return a list of machines that are not up to date with the control
// plane's configuration.
func (c *ControlPlane) NotUpToDateMachines() (collections.Machines, map[string][]string) {
	return c.machinesNotUptoDate, c.machinesNotUptoDateConditionMessages
}

// GetInfraResources fetches the external infrastructure resource for each machine in the collection
// and returns a map of machine.Name -> infraResource.
func GetInfraResources(ctx context.Context, cl client.Client, machines collections.Machines) (map[string]*unstructured.Unstructured, error) {
	result := map[string]*unstructured.Unstructured{}

	for _, m := range machines {
		infraObj, err := external.GetObjectFromContractVersionedRef(ctx, cl, m.Spec.InfrastructureRef, m.Namespace)
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
		if !bootstrapRef.IsDefined() {
			continue
		}

		machineConfig := &bootstrapv1.RKE2Config{}

		if err := cl.Get(ctx, client.ObjectKey{Name: bootstrapRef.Name, Namespace: m.Namespace}, machineConfig); err != nil {
			if apierrors.IsNotFound(errors.Cause(err)) {
				continue
			}

			if m.Status.NodeRef.IsDefined() {
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
			if err := helper.Patch(ctx, machine, patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
				controlplanev1.MachineAgentHealthyV1Beta1Condition,
				controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition,
				controlplanev1.NodeMetadataUpToDateV1Beta1Condition,
			}}, patch.WithOwnedConditions{Conditions: []string{
				controlplanev1.RKE2ControlPlaneMachineAgentHealthyCondition,
				controlplanev1.RKE2ControlPlaneMachineEtcdMemberHealthyCondition,
				controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition,
			}},
			); err != nil {
				if machine.Status.NodeRef.IsDefined() {
					_ = machine.Status.NodeRef.Name
				}

				errList = append(errList, err)
			}

			continue
		}

		if machine.Status.NodeRef.IsDefined() {
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

	ref := c.RCP.Spec.MachineTemplate.Spec.InfrastructureRef
	if !strings.HasSuffix(ref.Kind, clusterv1.TemplateSuffix) {
		return nil
	}

	obj, err := external.GetObjectFromContractVersionedRef(ctx, cl, ref, c.RCP.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.InfraMachineTemplateIsNotFound = true

			logger.Info(fmt.Sprintf("Could not find external object %s %s/%s.",
				obj.GroupVersionKind().Kind,
				obj.GetNamespace(),
				obj.GetName()))
		}

		return err
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

// UpToDate checks if a Machine is up to date with the control plane's configuration.
// If not, messages explaining why are provided with different level of detail for logs and conditions.
func UpToDate(ctx context.Context, machine *clusterv1.Machine, rcp *controlplanev1.RKE2ControlPlane,
	infraConfigs map[string]*unstructured.Unstructured, machineConfigs map[string]*bootstrapv1.RKE2Config,
) (bool, []string, []string, error) {
	logMessages := []string{}
	conditionMessages := []string{}

	// Machines that do not match with rcp config.
	matches, specLogMessages, specConditionMessages := matchesMachineSpec(ctx, infraConfigs, machineConfigs, rcp, machine)
	if !matches {
		logMessages = append(logMessages, specLogMessages...)
		conditionMessages = append(conditionMessages, specConditionMessages...)
	}

	if len(logMessages) > 0 || len(conditionMessages) > 0 {
		return false, logMessages, conditionMessages, nil
	}

	return true, nil, nil, nil
}

// matchesMachineSpec checks if a Machine matches any of a set of RKE2Configs and a set of infra machine configs.
// If it doesn't, it returns the reasons why.
// Kubernetes version, infrastructure template, and RKE2Config field need to be equivalent.
// Note: We don't need to compare the entire MachineSpec to determine if a Machine needs to be rolled out,
// because all the fields in the MachineSpec, except for version, the infrastructureRef and bootstrap.ConfigRef, are either:
// - mutated in-place (ex: NodeDrainTimeoutSeconds)
// - are not dictated by RCP (ex: ProviderID)
// - are not relevant for the rollout decision (ex: failureDomain).
func matchesMachineSpec(
	ctx context.Context,
	infraConfigs map[string]*unstructured.Unstructured,
	machineConfigs map[string]*bootstrapv1.RKE2Config,
	rcp *controlplanev1.RKE2ControlPlane,
	machine *clusterv1.Machine,
) (bool, []string, []string) {
	logMessages := []string{}
	conditionMessages := []string{}

	if !collections.MatchesKubernetesVersion(rcp.Spec.Version)(machine) {
		machineVersion := ""
		if machine != nil && machine.Spec.Version != "" {
			machineVersion = machine.Spec.Version
		}

		logMessages = append(logMessages, fmt.Sprintf("Machine version %q is not equal to RCP version %q", machineVersion, rcp.Spec.Version))
		// Note: the code computing the message for RCP's RolloutOut condition is making assumptions on the format/content of this message.
		conditionMessages = append(conditionMessages, fmt.Sprintf("Version %s, %s required", machineVersion, rcp.Spec.Version))
	}

	if matches := matchesRKE2BootstrapConfig(ctx, machineConfigs, rcp)(machine); !matches {
		logMessages = append(logMessages, "RKE2Config does not match RCP's RKE2ConfigSpec")
		conditionMessages = append(conditionMessages, "RKE2Config is not up-to-date")
	}

	if matches := matchesTemplateClonedFrom(ctx, infraConfigs, rcp)(machine); !matches {
		logMessages = append(logMessages, "Machine InfrastructureRef does not match RCP's MachineTemplate.InfrastructureRef")
		conditionMessages = append(conditionMessages, machine.Spec.InfrastructureRef.Kind+" is not up-to-date")
	}

	if len(logMessages) > 0 || len(conditionMessages) > 0 {
		return false, logMessages, conditionMessages
	}

	return true, nil, nil
}
