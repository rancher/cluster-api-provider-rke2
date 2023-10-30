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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RKE2ControlPlaneTemplateSpec defines the desired state of RKE2ControlPlaneTemplate.
type RKE2ControlPlaneTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of RKE2ControlPlaneTemplate. Edit rke2controlplanetemplate_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// RKE2ControlPlaneTemplateStatus defines the observed state of RKE2ControlPlaneTemplate.
type RKE2ControlPlaneTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// RKE2ControlPlaneTemplate is the Schema for the rke2controlplanetemplates API.
type RKE2ControlPlaneTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RKE2ControlPlaneTemplateSpec   `json:"spec,omitempty"`
	Status RKE2ControlPlaneTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RKE2ControlPlaneTemplateList contains a list of RKE2ControlPlaneTemplate.
type RKE2ControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RKE2ControlPlaneTemplate `json:"items"`
}

func init() { //nolint:gochecknoinits
	SchemeBuilder.Register(&RKE2ControlPlaneTemplate{}, &RKE2ControlPlaneTemplateList{})
}
