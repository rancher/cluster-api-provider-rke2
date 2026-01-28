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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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
