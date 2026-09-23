/*
Copyright 2025 SUSE LLC.

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
	"context"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
)

var _ = Describe("RKE2ControlPlane webhook", func() {
	var (
		oldRcp    *RKE2ControlPlane
		rcp       *RKE2ControlPlane
		defaulter = &RKE2ControlPlaneCustomDefaulter{}
		validator = &RKE2ControlPlaneCustomValidator{}
	)
	BeforeEach(func() {
		rcp = &RKE2ControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-control-plane",
				Namespace: "test",
			},
			Spec: RKE2ControlPlaneSpec{
				MachineTemplate: RKE2ControlPlaneMachineTemplate{
					Spec: RKE2ControlPlaneMachineTemplateSpec{
						InfrastructureRef: clusterv1.ContractVersionedObjectReference{
							Name: "foo",
						},
					},
				},
			},
		}
		oldRcp = rcp.DeepCopy()
		Expect(defaulter.Default(context.TODO(), rcp)).Should(Succeed())
		Expect(defaulter.Default(context.TODO(), oldRcp)).Should(Succeed())
	})
	It("Should not create RKE2ControlPlane with 0 replicas", func() {
		rcp.Spec.Replicas = nil
		_, err := validator.ValidateCreate(context.TODO(), rcp)
		Expect(err).Should(HaveOccurred())
		rcp.Spec.Replicas = ptr.To(int32(0))
		_, err = validator.ValidateCreate(context.TODO(), rcp)
		Expect(err).Should(HaveOccurred())
		rcp.Spec.Replicas = ptr.To(int32(1))
		_, err = validator.ValidateCreate(context.TODO(), rcp)
		Expect(err).ShouldNot(HaveOccurred())
	})
	It("Should not update RKE2ControlPlane with 0 replicas", func() {
		rcp.Spec.Replicas = nil
		_, err := validator.ValidateUpdate(context.TODO(), oldRcp, rcp)
		Expect(err).Should(HaveOccurred())
		rcp.Spec.Replicas = ptr.To(int32(0))
		_, err = validator.ValidateUpdate(context.TODO(), oldRcp, rcp)
		Expect(err).Should(HaveOccurred())
		rcp.Spec.Replicas = ptr.To(int32(1))
		_, err = validator.ValidateUpdate(context.TODO(), oldRcp, rcp)
		Expect(err).ShouldNot(HaveOccurred())
	})
})

func TestMachineTaintValidation(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		gateEnabled bool
		noTaints    bool
		wantErr     bool
	}{
		{name: "disabled without taints", noTaints: true},
		{name: "disabled with taints", key: "example.com/dedicated", wantErr: true},
		{name: "custom key", key: "example.com/dedicated", gateEnabled: true},
		{name: "control plane role", key: "node-role.kubernetes.io/control-plane", gateEnabled: true},
		{name: "out of service", key: "node.kubernetes.io/out-of-service", gateEnabled: true},
		{name: "uninitialized", key: clusterv1.NodeUninitializedTaint.Key, gateEnabled: true, wantErr: true},
		{name: "outdated revision", key: clusterv1.NodeOutdatedRevisionTaint.Key, gateEnabled: true, wantErr: true},
		{name: "node taint", key: "node.kubernetes.io/not-ready", gateEnabled: true, wantErr: true},
		{name: "cloud provider taint", key: "node.cloudprovider.kubernetes.io/uninitialized", gateEnabled: true, wantErr: true},
		{name: "deprecated role", key: "node-role.kubernetes.io/master", gateEnabled: true, wantErr: true},
		{name: "invalid key", key: "example.com/invalid/key", gateEnabled: true, wantErr: true},
		{name: "empty key", gateEnabled: true, wantErr: true},
		{name: "long key", key: "example.com/" + strings.Repeat("a", 64), gateEnabled: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			featuregatetesting.SetFeatureGateDuringTest(t, feature.Gates, feature.MachineTaintPropagation, tt.gateEnabled)
			rcp := &RKE2ControlPlane{
				Spec: RKE2ControlPlaneSpec{
					Replicas: ptr.To(int32(1)),
					MachineTemplate: RKE2ControlPlaneMachineTemplate{
						Spec: RKE2ControlPlaneMachineTemplateSpec{
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "test"},
						},
					},
				},
			}
			if !tt.noTaints {
				rcp.Spec.MachineTemplate.Spec.Taints = []clusterv1.MachineTaint{{
					Key: tt.key, Effect: "NoSchedule", Propagation: clusterv1.MachineTaintPropagationAlways,
				}}
			}
			rcpt := &RKE2ControlPlaneTemplate{
				Spec: RKE2ControlPlaneTemplateSpec{
					Template: RKE2ControlPlaneTemplateResource{
						Spec: RKE2ControlPlaneTemplateResourceSpec{MachineTemplate: rcp.Spec.MachineTemplate},
					},
				},
			}
			validator := &RKE2ControlPlaneCustomValidator{}
			templateValidator := &RKE2ControlPlaneTemplateCustomValidator{}
			_, createErr := validator.ValidateCreate(t.Context(), rcp)
			_, updateErr := validator.ValidateUpdate(t.Context(), &RKE2ControlPlane{}, rcp)
			_, templateCreateErr := templateValidator.ValidateCreate(t.Context(), rcpt)
			_, templateUpdateErr := templateValidator.ValidateUpdate(t.Context(), &RKE2ControlPlaneTemplate{}, rcpt)
			for _, err := range []error{createErr, updateErr, templateCreateErr, templateUpdateErr} {
				if tt.wantErr {
					g.Expect(err).To(MatchError(ContainSubstring("machineTemplate.spec.taints")))
				} else {
					g.Expect(err).NotTo(HaveOccurred())
				}
			}
		})
	}
}
