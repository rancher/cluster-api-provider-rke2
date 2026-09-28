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

package controllers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/infrastructure"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("cloneConfigsAndGenerateMachine naming", func() {
	var (
		ns      *corev1.Namespace
		cluster *clusterv1.Cluster
		rcp     *controlplanev1.RKE2ControlPlane
	)

	BeforeEach(func() {
		var err error
		ns, err = testEnv.CreateNamespace(ctx, "machine-naming")
		Expect(err).ToNot(HaveOccurred())

		cluster = &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: ns.Name,
			},
			Spec: clusterv1.ClusterSpec{},
		}

		infraTemplate := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": infrastructure.GroupVersion.String(),
				"kind":       "FakeMachineTemplate",
				"metadata": map[string]interface{}{
					"name":      "cp-infra",
					"namespace": ns.Name,
				},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{},
					},
				},
			},
		}
		Expect(testEnv.Create(ctx, infraTemplate)).To(Succeed())

		rcp = &controlplanev1.RKE2ControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster-control-plane",
				Namespace: ns.Name,
				UID:       "test-rcp-uid",
			},
			Spec: controlplanev1.RKE2ControlPlaneSpec{
				Replicas: ptr.To[int32](1),
				Version:  RKE2KubernetesVersion,
				MachineTemplate: controlplanev1.RKE2ControlPlaneMachineTemplate{
					Spec: controlplanev1.RKE2ControlPlaneMachineTemplateSpec{
						InfrastructureRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: infrastructure.GroupVersion.Group,
							Kind:     "FakeMachineTemplate",
							Name:     "cp-infra",
						},
					},
				},
			},
		}
	})

	AfterEach(func() {
		testEnv.Cleanup(ctx, ns)
	})

	It("uses the Machine name for InfraMachine and RKE2Config", func() {
		r := &RKE2ControlPlaneReconciler{
			Client: testEnv.GetClient(),
		}

		bootstrapSpec := &bootstrapv1.RKE2ConfigSpec{}
		Expect(r.cloneConfigsAndGenerateMachine(ctx, cluster, rcp, bootstrapSpec, "")).To(Succeed())

		machines := &clusterv1.MachineList{}
		Expect(testEnv.List(ctx, machines, client.InNamespace(ns.Name))).To(Succeed())
		Expect(machines.Items).To(HaveLen(1))
		machine := machines.Items[0]

		Expect(machine.Spec.InfrastructureRef.Name).To(Equal(machine.Name))
		Expect(machine.Spec.Bootstrap.ConfigRef.Name).To(Equal(machine.Name))

		infra := &unstructured.Unstructured{}
		infra.SetGroupVersionKind(infrastructure.GroupVersion.WithKind(infrastructure.FakeMachineKind))
		Expect(testEnv.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: machine.Name}, infra)).To(Succeed())

		config := &bootstrapv1.RKE2Config{}
		Expect(testEnv.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: machine.Name}, config)).To(Succeed())
	})
})
