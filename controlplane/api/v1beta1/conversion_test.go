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

package v1beta1

import (
	"testing"

	. "github.com/onsi/gomega"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
)

func TestMachineTaintsConversionRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		taints []clusterv1.MachineTaint
	}{
		{name: "without taints"},
		{
			name: "with taints",
			taints: []clusterv1.MachineTaint{
				{Key: "example.com/dedicated", Value: "control-plane", Effect: "NoSchedule", Propagation: clusterv1.MachineTaintPropagationAlways},
				{Key: "example.com/initializing", Effect: "NoExecute", Propagation: clusterv1.MachineTaintPropagationOnInitialization},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machineTemplate := controlplanev1.RKE2ControlPlaneMachineTemplate{
				Spec: controlplanev1.RKE2ControlPlaneMachineTemplateSpec{Taints: tt.taints},
			}
			t.Run("control plane", func(t *testing.T) {
				g := NewWithT(t)
				src := &controlplanev1.RKE2ControlPlane{
					Spec: controlplanev1.RKE2ControlPlaneSpec{MachineTemplate: machineTemplate},
				}
				spoke := &RKE2ControlPlane{}
				g.Expect(spoke.ConvertFrom(src)).To(Succeed())
				spoke.Spec.RegistrationAddress = "updated.example.com"
				dst := &controlplanev1.RKE2ControlPlane{}
				g.Expect(spoke.ConvertTo(dst)).To(Succeed())
				g.Expect(dst.Spec.MachineTemplate.Spec.Taints).To(Equal(tt.taints))
				g.Expect(dst.Spec.RegistrationAddress).To(Equal("updated.example.com"))
			})
			t.Run("control plane template", func(t *testing.T) {
				g := NewWithT(t)
				src := &controlplanev1.RKE2ControlPlaneTemplate{
					Spec: controlplanev1.RKE2ControlPlaneTemplateSpec{
						Template: controlplanev1.RKE2ControlPlaneTemplateResource{
							Spec: controlplanev1.RKE2ControlPlaneTemplateResourceSpec{MachineTemplate: machineTemplate},
						},
					},
				}
				spoke := &RKE2ControlPlaneTemplate{}
				g.Expect(spoke.ConvertFrom(src)).To(Succeed())
				spoke.Spec.Template.Spec.RegistrationAddress = "updated.example.com"
				dst := &controlplanev1.RKE2ControlPlaneTemplate{}
				g.Expect(spoke.ConvertTo(dst)).To(Succeed())
				g.Expect(dst.Spec.Template.Spec.MachineTemplate.Spec.Taints).To(Equal(tt.taints))
				g.Expect(dst.Spec.Template.Spec.RegistrationAddress).To(Equal("updated.example.com"))
			})
		})
	}
}
