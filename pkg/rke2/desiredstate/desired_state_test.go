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
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
)

func TestComputeDesiredMachineTaints(t *testing.T) {
	alwaysTaint := clusterv1.MachineTaint{
		Key:         "workload.example.com/dedicated",
		Value:       "control-plane",
		Effect:      corev1.TaintEffectPreferNoSchedule,
		Propagation: clusterv1.MachineTaintPropagationAlways,
	}
	initializationTaint := clusterv1.MachineTaint{
		Key:         "workload.example.com/initializing",
		Effect:      corev1.TaintEffectNoSchedule,
		Propagation: clusterv1.MachineTaintPropagationOnInitialization,
	}
	updatedTaint := alwaysTaint
	updatedTaint.Value = "updated"
	updatedTaint.Effect = corev1.TaintEffectNoSchedule
	updatedTaint.Propagation = clusterv1.MachineTaintPropagationOnInitialization

	tests := []struct {
		name          string
		existing      bool
		currentTaints []clusterv1.MachineTaint
		taints        []clusterv1.MachineTaint
	}{
		{
			name:   "new machine receives both propagation policies",
			taints: []clusterv1.MachineTaint{alwaysTaint, initializationTaint},
		},
		{
			name:          "existing machine receives added and updated taints",
			existing:      true,
			currentTaints: []clusterv1.MachineTaint{alwaysTaint},
			taints:        []clusterv1.MachineTaint{updatedTaint, initializationTaint},
		},
		{
			name:          "existing machine drops removed taints",
			existing:      true,
			currentTaints: []clusterv1.MachineTaint{alwaysTaint, initializationTaint},
			taints:        []clusterv1.MachineTaint{initializationTaint},
		},
		{
			name:          "omitted taints clear existing machine taints",
			existing:      true,
			currentTaints: []clusterv1.MachineTaint{alwaysTaint, initializationTaint},
		},
		{
			name:          "empty taints clear existing machine taints",
			existing:      true,
			currentTaints: []clusterv1.MachineTaint{alwaysTaint, initializationTaint},
			taints:        []clusterv1.MachineTaint{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			rcp := &controlplanev1.RKE2ControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "control-plane",
					Namespace: "test",
					UID:       "control-plane-uid",
				},
				Spec: controlplanev1.RKE2ControlPlaneSpec{
					Version: "v1.35.2+rke2r1",
				},
			}
			rcp.Spec.MachineTemplate.Spec.Taints = tt.taints
			cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: rcp.Namespace}}
			infraRef := clusterv1.ContractVersionedObjectReference{
				APIGroup: "infrastructure.cluster.x-k8s.io",
				Kind:     "DockerMachine",
				Name:     "machine-infrastructure",
			}
			bootstrapRef := clusterv1.ContractVersionedObjectReference{
				APIGroup: "bootstrap.cluster.x-k8s.io",
				Kind:     "RKE2Config",
				Name:     "machine-bootstrap",
			}
			var existingMachine *clusterv1.Machine
			if tt.existing {
				existingMachine = &clusterv1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-machine",
						Namespace: rcp.Namespace,
						UID:       "existing-machine-uid",
					},
					Spec: clusterv1.MachineSpec{
						Version:           "v1.34.5+rke2r1",
						FailureDomain:     "zone-a",
						InfrastructureRef: infraRef,
						Bootstrap:         clusterv1.Bootstrap{ConfigRef: bootstrapRef},
						Taints:            tt.currentTaints,
					},
				}
			}
			originalMachine := existingMachine.DeepCopy()

			desiredMachine, err := ComputeDesiredMachine(rcp, cluster, infraRef, bootstrapRef, "zone-a", existingMachine)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(desiredMachine.Spec.Taints).To(Equal(tt.taints))
			g.Expect(desiredMachine.Spec.InfrastructureRef).To(Equal(infraRef))
			g.Expect(desiredMachine.Spec.Bootstrap.ConfigRef).To(Equal(bootstrapRef))
			g.Expect(desiredMachine.Spec.FailureDomain).To(Equal("zone-a"))
			g.Expect(desiredMachine.Namespace).To(Equal(rcp.Namespace))
			g.Expect(existingMachine).To(Equal(originalMachine))

			if existingMachine == nil {
				g.Expect(desiredMachine.Name).To(HavePrefix(rcp.Name + "-"))
				g.Expect(desiredMachine.Spec.Version).To(Equal(rcp.Spec.Version))
			} else {
				g.Expect(desiredMachine.Name).To(Equal(existingMachine.Name))
				g.Expect(desiredMachine.UID).To(Equal(existingMachine.UID))
				g.Expect(desiredMachine.Spec.Version).To(Equal(existingMachine.Spec.Version))
			}
		})
	}
}
