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

package controllers

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
)

var _ = Describe("RKE2Config Scope", func() {
	It("should initialize scope", func() {
		ctx := context.Background()
		scheme := runtime.NewScheme()
		Expect(clusterv1.AddToScheme(scheme)).Should(Succeed())
		Expect(bootstrapv1.AddToScheme(scheme)).Should(Succeed())
		Expect(controlplanev1.AddToScheme(scheme)).Should(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		configKey := types.NamespacedName{Namespace: "default", Name: "test-config"}
		request := ctrl.Request{NamespacedName: configKey}

		By("returning not found error if config is not found")
		_, err := NewScope(ctx, request, fakeClient)
		Expect(errors.Is(err, ErrRKE2ConfigNotFound)).Should(BeTrue())

		By("returning no owner error if config has no owners")
		config := &bootstrapv1.RKE2Config{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configKey.Name,
				Namespace: configKey.Namespace,
			},
		}
		Expect(fakeClient.Create(ctx, config)).Should(Succeed())
		_, err = NewScope(ctx, request, fakeClient)
		Expect(errors.Is(err, ErrNoRKE2ConfigOwner)).Should(BeTrue())

		By("initializing a scope for a MachinePool owner")
		k8sVersion := "v1.2.3+test"
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: configKey.Namespace,
			},
		}
		Expect(fakeClient.Create(ctx, cluster)).Should(Succeed())

		machinePool := &clusterv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machinepool",
				Namespace: configKey.Namespace,
			},
			Spec: clusterv1.MachinePoolSpec{
				ClusterName: cluster.Name,
				Template: clusterv1.MachineTemplateSpec{
					Spec: clusterv1.MachineSpec{
						Version: k8sVersion,
					},
				},
			},
		}
		Expect(fakeClient.Create(ctx, machinePool)).Should(Succeed())

		config.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "MachinePool",
				Name:       machinePool.Name,
				UID:        types.UID("foo"),
				Controller: ptr.To(true),
			},
		}
		Expect(fakeClient.Update(ctx, config)).Should(Succeed())

		scope, err := NewScope(ctx, request, fakeClient)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(scope.HasMachinePoolOwner()).Should(BeTrue())
		Expect(scope.MachinePool).ShouldNot(BeNil())
		Expect(scope.MachinePool).Should(Equal(machinePool))

		By("initializing a scope for a Machine owner")
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: configKey.Namespace,
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: cluster.Name,
				Version:     k8sVersion,
			},
		}
		Expect(fakeClient.Create(ctx, machine)).Should(Succeed())

		config.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Machine",
				Name:       machine.Name,
				UID:        types.UID("foo"),
				Controller: ptr.To(true),
			},
		}
		Expect(fakeClient.Update(ctx, config)).Should(Succeed())

		scope, err = NewScope(ctx, request, fakeClient)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(scope.HasMachineOwner()).Should(BeTrue())
		Expect(scope.Machine).ShouldNot(BeNil())
		Expect(scope.Machine).Should(Equal(machine))
		Expect(scope.HasControlPlaneOwner()).Should(BeFalse())

		By("initializing a scope for a ControlPlane Machine owner")
		controlPlane := &controlplanev1.RKE2ControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-control-plane",
				Namespace: configKey.Namespace,
			},
		}
		Expect(fakeClient.Create(ctx, controlPlane)).Should(Succeed())

		machine.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: controlplanev1.GroupVersion.String(),
				Kind:       "RKE2ControlPlane",
				Name:       controlPlane.Name,
				UID:        types.UID("foo"),
				Controller: ptr.To(true),
			},
		}
		Expect(fakeClient.Update(ctx, machine)).Should(Succeed())

		scope, err = NewScope(ctx, request, fakeClient)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(scope.HasControlPlaneOwner()).Should(BeTrue())
		Expect(scope.ControlPlane).ShouldNot(BeNil())
		Expect(scope.ControlPlane).Should(Equal(controlPlane))
	})
})
