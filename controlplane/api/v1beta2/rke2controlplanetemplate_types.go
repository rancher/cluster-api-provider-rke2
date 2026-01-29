/*
Copyright 2026 SUSE LLC.

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

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
)

// RKE2ControlPlaneTemplateSpec defines the desired state of RKE2ControlPlaneTemplate.
type RKE2ControlPlaneTemplateSpec struct {
	// template defines the desired state of RKE2ControlPlaneTemplate.
	// +required
	Template RKE2ControlPlaneTemplateResource `json:"template"`
}

// RKE2ControlPlaneTemplateResource contains spec for RKE2ControlPlaneTemplate.
type RKE2ControlPlaneTemplateResource struct {
	// Spec is the specification of the desired behavior of the control plane.
	Spec RKE2ControlPlaneTemplateResourceSpec `json:"spec"`
}

// RKE2ControlPlaneTemplateResourceSpec contains the spec for RKE2ControlPlaneTemplateResource.
type RKE2ControlPlaneTemplateResourceSpec struct {
	// RKE2AgentSpec contains the node spec for the RKE2 Control plane nodes.
	bootstrapv1.RKE2ConfigSpec `json:",inline"`

	// MachineTemplate contains information about how machines
	// should be shaped when creating or updating a control plane.
	// +optional
	MachineTemplate RKE2ControlPlaneMachineTemplate `json:"machineTemplate,omitempty,omitzero"`

	// ServerConfig specifies configuration for the agent nodes.
	//+optional
	ServerConfig RKE2ServerConfig `json:"serverConfig,omitempty,omitzero"`

	// ManifestsConfigMapReference references a ConfigMap which contains Kubernetes manifests to be deployed automatically on the cluster
	// Each data entry in the ConfigMap will be will be copied to a folder on the control plane nodes that RKE2 scans and uses to deploy manifests.
	//+optional
	ManifestsConfigMapReference corev1.ObjectReference `json:"manifestsConfigMapReference,omitempty,omitzero"`

	// RegistrationMethod is the method to use for registering nodes into the RKE2 cluster.
	// +kubebuilder:validation:Enum=internal-first;internal-only-ips;external-only-ips;address;control-plane-endpoint;""
	// +optional
	RegistrationMethod RegistrationMethod `json:"registrationMethod,omitempty,omitzero"`

	// RegistrationAddress is an explicit address to use when registering a node. This is required if
	// the registration type is "address". Its for scenarios where a load-balancer or VIP is used.
	// +optional
	RegistrationAddress string `json:"registrationAddress,omitempty"`

	// The Rollout to use to replace control plane machines with new ones.
	RolloutStrategy *RolloutStrategy `json:"rolloutStrategy"`

	// remediationStrategy is the RemediationStrategy that controls how control plane machine remediation happens.
	// +optional
	RemediationStrategy *RemediationStrategy `json:"remediationStrategy,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=rke2controlplanetemplates,scope=Namespaced,categories=cluster-api,shortName=rke2ct
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="ClusterClass",type="string", JSONPath=`.metadata.ownerReferences[?(@.kind=="ClusterClass")].name`,description="Name of the ClusterClass owning this template"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of RKE2ControlPlaneTemplate"

// RKE2ControlPlaneTemplate is the Schema for the rke2controlplanetemplates API.
type RKE2ControlPlaneTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// Spec is the control plane specification for the template resource.
	Spec RKE2ControlPlaneTemplateSpec `json:"spec,omitzero"`
}

//+kubebuilder:object:root=true

// RKE2ControlPlaneTemplateList contains a list of RKE2ControlPlaneTemplate.
type RKE2ControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`

	Items []RKE2ControlPlaneTemplate `json:"items"`
}

func init() { //nolint:gochecknoinits
	objectTypes = append(objectTypes, &RKE2ControlPlaneTemplate{}, &RKE2ControlPlaneTemplateList{})
}
