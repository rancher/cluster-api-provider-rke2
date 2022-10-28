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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Rke2ControlPlaneTemplateSpec defines the desired state of Rke2ControlPlaneTemplate
type Rke2ControlPlaneTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Rke2ControlPlaneTemplate. Edit rke2controlplanetemplate_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// Rke2ControlPlaneTemplateStatus defines the observed state of Rke2ControlPlaneTemplate
type Rke2ControlPlaneTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Rke2ControlPlaneTemplate is the Schema for the rke2controlplanetemplates API
type Rke2ControlPlaneTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   Rke2ControlPlaneTemplateSpec   `json:"spec,omitempty"`
	Status Rke2ControlPlaneTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// Rke2ControlPlaneTemplateList contains a list of Rke2ControlPlaneTemplate
type Rke2ControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rke2ControlPlaneTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Rke2ControlPlaneTemplate{}, &Rke2ControlPlaneTemplateList{})
}
