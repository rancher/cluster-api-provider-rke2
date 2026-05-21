/*
Copyright 2026 SUSE.

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

package desiredstate

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/storage/names"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	rke2util "github.com/rancher/cluster-api-provider-rke2/pkg/util"
)

// ControlPlaneMachineLabels returns a set of labels to add to a control plane machine for this specific cluster.
func ControlPlaneMachineLabels(rcp *controlplanev1.RKE2ControlPlane, clusterName string) map[string]string {
	labels := map[string]string{}

	// Add the labels from the MachineTemplate.
	// Note: we intentionally don't use the map directly to ensure we don't modify the map in RCP.
	maps.Copy(labels, rcp.Spec.MachineTemplate.ObjectMeta.Labels)

	// Always force these labels over the ones coming from the spec.
	labels[clusterv1.ClusterNameLabel] = clusterName
	labels[clusterv1.MachineControlPlaneLabel] = ""
	labels[clusterv1.MachineControlPlaneNameLabel] = rcp.Name

	return labels
}

// ControlPlaneMachineAnnotations returns a set of annotations to add to a control plane machine for this specific cluster.
func ControlPlaneMachineAnnotations(rcp *controlplanev1.RKE2ControlPlane) map[string]string {
	annotations := map[string]string{}

	// Add the annotations from the MachineTemplate.
	// Note: we intentionally don't use the map directly to ensure we don't modify the map in RCP.
	maps.Copy(annotations, rcp.Spec.MachineTemplate.ObjectMeta.Annotations)

	return annotations
}

// ComputeDesiredMachine computes the desired Machine.
// This Machine will be used during reconciliation to:
// * create a new Machine
// * update an existing Machine
// Because we are using Server-Side-Apply we always have to calculate the full object.
// There are small differences in how we calculate the Machine depending on if it
// is a create or update. Example: for a new Machine we have to calculate a new name,
// while for an existing Machine we have to use the name of the existing Machine.
func ComputeDesiredMachine(
	rcp *controlplanev1.RKE2ControlPlane,
	cluster *clusterv1.Cluster,
	infraRef, bootstrapRef clusterv1.ContractVersionedObjectReference,
	failureDomain string,
	existingMachine *clusterv1.Machine,
) (*clusterv1.Machine, error) {
	var (
		machineName string
		machineUID  types.UID
		version     string
	)

	annotations := map[string]string{}

	if existingMachine == nil {
		// Creating a new machine
		machineName = names.SimpleNameGenerator.GenerateName(rcp.Name + "-")
		version = rcp.GetDesiredVersion()

		serverConfig, err := json.Marshal(rcp.Spec.ServerConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal server configuration: %w", err)
		}

		annotations[controlplanev1.RKE2ServerConfigurationAnnotation] = string(serverConfig)
		// Setting pre-terminate hook so we can later remove the etcd member right before Machine termination
		// (i.e. before InfraMachine deletion).
		annotations[controlplanev1.PreTerminateHookCleanupAnnotation] = ""
	} else {
		// Updating an existing machine
		machineName = existingMachine.Name
		machineUID = existingMachine.UID
		version = existingMachine.Spec.Version

		if serverConfig, ok := existingMachine.Annotations[controlplanev1.RKE2ServerConfigurationAnnotation]; ok {
			annotations[controlplanev1.RKE2ServerConfigurationAnnotation] = serverConfig
		}
	}

	desiredMachine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			UID:       machineUID,
			Name:      machineName,
			Namespace: rcp.Namespace,
			// Note: by setting the ownerRef on creation we signal to the Machine controller that this is not a stand-alone Machine.
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(rcp, controlplanev1.GroupVersion.WithKind("RKE2ControlPlane")),
			},
			Labels:      ControlPlaneMachineLabels(rcp, cluster.Name),
			Annotations: map[string]string{},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       cluster.Name,
			Version:           version,
			FailureDomain:     failureDomain,
			InfrastructureRef: infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
		},
	}

	// Set the in-place mutable fields.
	// When we create a new Machine we will just create the Machine with those fields.
	// When we update an existing Machine will we update the fields on the existing Machine (in-place mutate).

	// Set annotations
	maps.Copy(desiredMachine.Annotations, rcp.Spec.MachineTemplate.ObjectMeta.Annotations)
	maps.Copy(desiredMachine.Annotations, annotations)

	// Set other in-place mutable fields
	desiredMachine.Spec.Deletion = clusterv1.MachineDeletionSpec{
		NodeDrainTimeoutSeconds:        rcp.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds,
		NodeDeletionTimeoutSeconds:     rcp.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds,
		NodeVolumeDetachTimeoutSeconds: rcp.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds,
	}

	return desiredMachine, nil
}

// ComputeDesiredRKE2Config computes the desired RKE2Config.
func ComputeDesiredRKE2Config(
	rcp *controlplanev1.RKE2ControlPlane,
	cluster *clusterv1.Cluster,
	name string,
	existingRKE2Config *bootstrapv1.RKE2Config,
) (*bootstrapv1.RKE2Config, error) {
	// Create an owner reference without a controller reference because the owning controller is the machine controller
	var ownerReferences []metav1.OwnerReference
	if existingRKE2Config == nil || !util.HasOwner(existingRKE2Config.OwnerReferences, clusterv1.GroupVersion.String(), []string{"Machine"}) {
		ownerReferences = append(ownerReferences, metav1.OwnerReference{
			APIVersion: controlplanev1.GroupVersion.String(),
			Kind:       "RKE2ControlPlane",
			Name:       rcp.Name,
			UID:        rcp.UID,
		})
	}

	rke2Config := &bootstrapv1.RKE2Config{
		TypeMeta: metav1.TypeMeta{
			APIVersion: bootstrapv1.GroupVersion.String(),
			Kind:       "RKE2Config",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       rcp.Namespace,
			Labels:          ControlPlaneMachineLabels(rcp, cluster.Name),
			Annotations:     ControlPlaneMachineAnnotations(rcp),
			OwnerReferences: ownerReferences,
		},
		Spec: *rcp.Spec.RKE2ConfigSpec.DeepCopy(),
	}

	if existingRKE2Config != nil {
		rke2Config.SetName(existingRKE2Config.GetName())
		rke2Config.SetUID(existingRKE2Config.GetUID())
	}

	return rke2Config, nil
}

// ComputeDesiredInfraMachine computes the desired InfraMachine.
func ComputeDesiredInfraMachine(
	ctx context.Context,
	c client.Client,
	rcp *controlplanev1.RKE2ControlPlane,
	cluster *clusterv1.Cluster,
	name string,
	existingInfraMachine *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	// Create an owner reference without a controller reference because the owning controller is the machine controller
	var ownerReference *metav1.OwnerReference
	if existingInfraMachine == nil || !util.HasOwner(existingInfraMachine.GetOwnerReferences(), clusterv1.GroupVersion.String(), []string{"Machine"}) {
		ownerReference = &metav1.OwnerReference{
			APIVersion: controlplanev1.GroupVersion.String(),
			Kind:       "RKE2ControlPlane",
			Name:       rcp.Name,
			UID:        rcp.UID,
		}
	}

	infraRef := rcp.Spec.MachineTemplate.Spec.InfrastructureRef

	apiVersion, err := rke2util.GetAPIVersion(ctx, c, infraRef.GroupKind())
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute desired InfraMachine")
	}

	templateRef := &corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       infraRef.Kind,
		Name:       infraRef.Name,
		Namespace:  rcp.Namespace,
	}

	template, err := external.Get(ctx, c, templateRef)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute desired InfraMachine")
	}

	infraMachine, err := external.GenerateTemplate(&external.GenerateTemplateInput{
		Template:    template,
		TemplateRef: templateRef,
		Namespace:   rcp.Namespace,
		Name:        name,
		ClusterName: cluster.Name,
		OwnerRef:    ownerReference,
		Labels:      ControlPlaneMachineLabels(rcp, cluster.Name),
		Annotations: ControlPlaneMachineAnnotations(rcp),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute desired InfraMachine")
	}

	if existingInfraMachine != nil {
		infraMachine.SetName(existingInfraMachine.GetName())
		infraMachine.SetUID(existingInfraMachine.GetUID())
	}

	return infraMachine, nil
}
