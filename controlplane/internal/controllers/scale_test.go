package controllers

import (
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
)

func TestComputeDesiredMachineTaints(t *testing.T) {
	taints := []clusterv1.MachineTaint{
		{Key: "dedicated", Value: "control-plane", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways},
		{Key: "initializing", Effect: corev1.TaintEffectPreferNoSchedule, Propagation: clusterv1.MachineTaintPropagationOnInitialization},
	}
	for _, tt := range []struct {
		name     string
		existing *clusterv1.Machine
		desired  []clusterv1.MachineTaint
	}{
		{name: "new machine", desired: taints},
		{
			name: "existing machine",
			existing: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "existing", UID: "original"},
				Spec:       clusterv1.MachineSpec{Version: "v1.34.2+rke2r1"},
			},
			desired: taints,
		},
		{
			name: "remove taints",
			existing: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "existing", UID: "original"},
				Spec:       clusterv1.MachineSpec{Version: "v1.34.2+rke2r1", Taints: taints},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			rcp := &controlplanev1.RKE2ControlPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "control-plane", Namespace: "default"},
				Spec: controlplanev1.RKE2ControlPlaneSpec{
					Version: "v1.35.0+rke2r1",
					MachineTemplate: controlplanev1.RKE2ControlPlaneMachineTemplate{
						Spec: controlplanev1.RKE2ControlPlaneMachineTemplateSpec{Taints: tt.desired},
					},
				},
			}
			cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
			r := &RKE2ControlPlaneReconciler{}
			machine, err := r.computeDesiredMachine(rcp, cluster,
				clusterv1.ContractVersionedObjectReference{}, clusterv1.ContractVersionedObjectReference{}, "", tt.existing)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(machine.Spec.Taints).To(Equal(tt.desired))
			if tt.existing != nil {
				g.Expect(machine.Name).To(Equal(tt.existing.Name))
				g.Expect(machine.UID).To(Equal(tt.existing.UID))
				g.Expect(machine.Spec.Version).To(Equal(tt.existing.Spec.Version))
			}
		})
	}
}
