package rke2

import (
	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1alpha1"
	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
)

// matchesRCPConfiguration returns a filter to find all machines that matches with RCP config and do not require any rollout.
// Kubernetes version, infrastructure template, and RKE2Config field need to be equivalent.
func matchesRCPConfiguration(infraConfigs map[string]*unstructured.Unstructured, machineConfigs map[string]*bootstrapv1.RKE2Config, rcp *controlplanev1.RKE2ControlPlane) func(machine *clusterv1.Machine) bool {
	return collections.And(
		collections.MatchesKubernetesVersion(rcp.Spec.Version),
		matchesRKE2BootstrapConfig(machineConfigs, rcp),
		matchesTemplateClonedFrom(infraConfigs, rcp),
	)
}

// matchesRKE2BootstrapConfig checks if machine's RKE2ConfigSpec is equivalent with RCP's RKE2ConfigSpec.
func matchesRKE2BootstrapConfig(machineConfigs map[string]*bootstrapv1.RKE2Config, rcp *controlplanev1.RKE2ControlPlane) collections.Func {
	return func(machine *clusterv1.Machine) bool {
		return true // TODO: implement by using inspiration from https://github.com/kubernetes-sigs/cluster-api/blob/v1.3.2/controlplane/kubeadm/internal/filters.go#L81
	}
}

// matchesTemplateClonedFrom returns a filter to find all machines that match a given RCP infra template.
func matchesTemplateClonedFrom(infraConfigs map[string]*unstructured.Unstructured, rcp *controlplanev1.RKE2ControlPlane) collections.Func {
	return func(machine *clusterv1.Machine) bool {

		if machine == nil {
			return false
		}
		infraObj, found := infraConfigs[machine.Name]
		if !found {
			// Return true here because failing to get infrastructure machine should not be considered as unmatching.
			return true
		}

		clonedFromName, ok1 := infraObj.GetAnnotations()[clusterv1.TemplateClonedFromNameAnnotation]
		clonedFromGroupKind, ok2 := infraObj.GetAnnotations()[clusterv1.TemplateClonedFromGroupKindAnnotation]
		if !ok1 || !ok2 {
			// All rcp cloned infra machines should have this annotation.
			// Missing the annotation may be due to older version machines or adopted machines.
			// Should not be considered as mismatch.
			return true
		}

		// Check if the machine's infrastructure reference has been created from the current RCP infrastructure template.
		if clonedFromName != rcp.Spec.InfrastructureRef.Name ||
			clonedFromGroupKind != rcp.Spec.InfrastructureRef.GroupVersionKind().GroupKind().String() {
			return false
		}
		return true
	}
}
