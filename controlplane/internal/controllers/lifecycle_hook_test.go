/*
Copyright 2025 SUSE.

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/infrastructure"
	"github.com/rancher/cluster-api-provider-rke2/pkg/rke2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

var _ = Describe("Lifecycle Hooks", Ordered, func() {
	var (
		err             error
		ns              *corev1.Namespace
		cp              *rke2.ControlPlane
		rcp             *controlplanev1.RKE2ControlPlane
		machine         clusterv1.Machine
		spareMachine    clusterv1.Machine
		node            *corev1.Node
		cluster         *clusterv1.Cluster
		r               *RKE2ControlPlaneReconciler
		m               *rke2.Management
		machineStatus   clusterv1.MachineStatus
		workloadCluster rke2.WorkloadCluster
		clusterCache    clustercache.ClusterCache
	)
	BeforeAll(func() {
		ns, err = testEnv.CreateNamespace(ctx, "test-hooks")
		Expect(err).ShouldNot(HaveOccurred())

		cluster = &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: ns.Name,
			},
			Spec: clusterv1.ClusterSpec{
				ClusterNetwork: clusterv1.ClusterNetwork{
					Pods: clusterv1.NetworkRanges{
						CIDRBlocks: []string{
							"192.168.0.0/16",
						},
					},
					Services: clusterv1.NetworkRanges{
						CIDRBlocks: []string{
							"192.169.0.0/16",
						},
					},
				},
			},
		}
		Expect(testEnv.Client.Create(ctx, cluster)).To(Succeed())

		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "true",
				},
				Annotations: map[string]string{
					clusterv1.MachineAnnotation: "test-node",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				}},
			},
		}
		Expect(testEnv.Create(ctx, node.DeepCopy())).To(Succeed())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(node), node)).Should(Succeed())
		Expect(testEnv.Status().Update(ctx, node.DeepCopy())).To(Succeed())

		machineStatus = clusterv1.MachineStatus{
			NodeRef: clusterv1.MachineNodeReference{
				Name: node.Name,
			},
			Conditions: []metav1.Condition{
				{
					Type:               clusterv1.ReadyCondition,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
				},
			},
		}

		machine = clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Finalizers:  []string{"test/finalizer"},
				Name:        "test-machine",
				Namespace:   ns.Name,
				Annotations: map[string]string{controlplanev1.PreDrainLoadbalancerExclusionAnnotation: "test"},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: cluster.Name,
				Bootstrap: clusterv1.Bootstrap{
					DataSecretName: ptr.To("dummy-secret"),
				},
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "FakeMachine",
					APIGroup: infrastructure.GroupVersion.Group,
					Name:     "fakem1",
				},
			},
			Status: machineStatus,
		}

		spareMachine = clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Finalizers:  []string{"test/finalizer"},
				Name:        "another-machine",
				Namespace:   ns.Name,
				Annotations: map[string]string{controlplanev1.PreDrainLoadbalancerExclusionAnnotation: "test"},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: cluster.Name,
				Bootstrap: clusterv1.Bootstrap{
					DataSecretName: ptr.To("dummy-secret"),
				},
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "FakeMachine",
					APIGroup: infrastructure.GroupVersion.Group,
					Name:     "fakemspare1",
				},
			},
			Status: clusterv1.MachineStatus{
				NodeRef: clusterv1.MachineNodeReference{
					Name: node.Name,
				},
			},
		}

		Expect(testEnv.Create(ctx, &machine)).To(Succeed())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(&machine), &machine)).Should(Succeed())
		Expect(testEnv.Status().Update(ctx, &machine)).To(Succeed())

		Expect(testEnv.Create(ctx, &spareMachine)).To(Succeed())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(&spareMachine), &spareMachine)).Should(Succeed())
		Expect(testEnv.Status().Update(ctx, &spareMachine)).To(Succeed())

		clusterCache, err = clustercache.SetupWithManager(ctx, testEnv.Manager, clustercache.Options{
			SecretClient: testEnv.GetClient(),
			Client: clustercache.ClientOptions{
				UserAgent: remote.DefaultClusterAPIUserAgent("test-controller-manager"),
				Cache: clustercache.ClientCacheOptions{
					DisableFor: []client.Object{
						// Don't cache ConfigMaps & Secrets.
						&corev1.ConfigMap{},
						&corev1.Secret{},
					},
				},
			},
		}, controller.Options{MaxConcurrentReconciles: 10, SkipNameValidation: ptr.To(true)})
		Expect(err).ToNot(HaveOccurred())

		ml := clusterv1.MachineList{Items: []clusterv1.Machine{machine, spareMachine}}

		rcp = &controlplanev1.RKE2ControlPlane{
			Status: controlplanev1.RKE2ControlPlaneStatus{
				Initialization: controlplanev1.RKE2ControlPlaneInitializationStatus{
					ControlPlaneInitialized: ptr.To(true),
				},
			},
		}

		m = &rke2.Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
			ClusterCache:        clusterCache,
		}

		cp, err = rke2.NewControlPlane(ctx, m, testEnv.GetClient(), cluster, rcp, collections.FromMachineList(&ml))
		Expect(err).ToNot(HaveOccurred())

		ref := metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       clusterv1.ClusterKind,
			UID:        cp.Cluster.GetUID(),
			Name:       cp.Cluster.GetName(),
		}
		Expect(testEnv.Client.Create(ctx, kubeconfig.GenerateSecretWithOwner(
			client.ObjectKeyFromObject(cp.Cluster),
			kubeconfig.FromEnvTestConfig(testEnv.Config, cp.Cluster),
			ref))).To(Succeed())

		r = &RKE2ControlPlaneReconciler{
			Client:                    testEnv.GetClient(),
			Scheme:                    testEnv.GetScheme(),
			managementCluster:         &rke2.Management{Client: testEnv.GetClient(), SecretCachingClient: testEnv.GetClient()},
			managementClusterUncached: &rke2.Management{Client: testEnv.GetClient()},
		}
	})
	AfterAll(func() {
		machine.Finalizers = []string{}
		Expect(testEnv.Update(ctx, &machine)).Should(Succeed())
		testEnv.Cleanup(ctx, node, &machine, &spareMachine, cluster, ns)
	})
	It("Should cleanup load balancer exclusion annotation", func() {
		_, found := machine.Annotations[controlplanev1.PreDrainLoadbalancerExclusionAnnotation]
		Expect(found).Should(BeTrue(), "pre-drain annotation should have been set during test initialization")

		result, err := r.reconcileLifecycleHooks(ctx, cp)
		Expect(result.IsZero()).Should(BeTrue())
		Expect(err).ShouldNot(HaveOccurred())

		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(&machine), &machine)).Should(Succeed())
		_, found = machine.Annotations[controlplanev1.PreDrainLoadbalancerExclusionAnnotation]
		Expect(found).Should(BeFalse(), "pre-drain annotation should have been cleaned since feature is turned off")
	})
	It("Should have added the pre-terminate hook annotation", func() {
		_, found := machine.Annotations[controlplanev1.PreTerminateHookCleanupAnnotation]
		Expect(found).Should(BeTrue(), "pre-terminate annotation should have been added already")
	})
	It("Should add load balancer exclusion annotation if feature is enabled", func() {
		if rcp.Annotations == nil {
			rcp.Annotations = map[string]string{}
		}
		rcp.Annotations[controlplanev1.LoadBalancerExclusionAnnotation] = "true"

		result, err := r.reconcileLifecycleHooks(ctx, cp)
		Expect(result.IsZero()).Should(BeTrue())
		Expect(err).ShouldNot(HaveOccurred())

		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(&machine), &machine)).Should(Succeed())
		_, found := machine.Annotations[controlplanev1.PreDrainLoadbalancerExclusionAnnotation]
		Expect(found).Should(BeTrue(), "pre-drain annotation should have been added")
	})
	It("Should reconcile pre-drain hooks and label the node with load balancer exclusion on machine deletion", func() {
		Expect(testEnv.Delete(ctx, &machine)).Should(Succeed())
		Eventually(func() bool {
			Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(&machine), &machine)).Should(Succeed())
			return machine.DeletionTimestamp.IsZero()
		}).WithTimeout(10*time.Second).Should(BeFalse(), "machine should have a deletion timestamp")
		machine.Status = clusterv1.MachineStatus{
			NodeRef: clusterv1.MachineNodeReference{
				Name: node.Name,
			},
			Conditions: []metav1.Condition{
				{
					Type:               clusterv1.ReadyCondition,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
				},
				{
					Type:               clusterv1.MachineDeletingCondition,
					Status:             metav1.ConditionTrue,
					Reason:             clusterv1.MachineDeletingWaitingForPreDrainHookReason,
					LastTransitionTime: metav1.Now(),
				},
			},
		}

		// Actualize the machine list since they were modified externally
		ml := clusterv1.MachineList{Items: []clusterv1.Machine{machine, spareMachine}}
		cp, err = rke2.NewControlPlane(ctx, m, testEnv.GetClient(), cluster, rcp, collections.FromMachineList(&ml))
		Expect(err).ToNot(HaveOccurred())
		workloadCluster, err = cp.GetWorkloadCluster(ctx)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(workloadCluster.InitWorkload(ctx, cp)).Should(Succeed())

		// Validate pre-drain hook
		result, err := r.reconcileLifecycleHooks(ctx, cp)
		Expect(result.IsZero()).Should(BeFalse())
		Expect(err).ShouldNot(HaveOccurred())

		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(node), node)).Should(Succeed())
		Expect(node.Labels).ShouldNot(BeNil())
		value, found := node.Labels[corev1.LabelNodeExcludeBalancers]
		Expect(found).Should(BeTrue(), "node must have label")
		Expect(value).Should(Equal("true"))

		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(&machine), &machine)).Should(Succeed())
		_, found = machine.Annotations[controlplanev1.PreDrainLoadbalancerExclusionAnnotation]
		Expect(found).Should(BeFalse(), "pre-drain annotation should have been deleted")
	})

	It("Should reconcile pre-terminate hooks and remove pre-terminate annotation on deletion", func() {
		Expect(testEnv.Delete(ctx, &machine)).Should(Succeed())
		Eventually(func() bool {
			Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(&machine), &machine)).Should(Succeed())
			return machine.DeletionTimestamp.IsZero()
		}).WithTimeout(10*time.Second).Should(BeFalse(), "machine should have a deletion timestamp")
		machine.Status.Conditions = []metav1.Condition{
			{
				Type:               clusterv1.MachineDeletingCondition,
				Status:             metav1.ConditionTrue,
				Reason:             clusterv1.MachineDeletingWaitingForPreTerminateHookReason,
				LastTransitionTime: metav1.Now(),
			},
		}

		// Actualize the machine list since they were modified externally
		ml := clusterv1.MachineList{Items: []clusterv1.Machine{machine, spareMachine}}
		cp, err = rke2.NewControlPlane(ctx, m, testEnv.GetClient(), cluster, rcp, collections.FromMachineList(&ml))
		Expect(err).ToNot(HaveOccurred())
		workloadCluster, err = cp.GetWorkloadCluster(ctx)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(workloadCluster.InitWorkload(ctx, cp)).Should(Succeed())

		result, err := r.reconcileLifecycleHooks(ctx, cp)
		Expect(result.IsZero()).Should(BeFalse())
		Expect(err).ShouldNot(HaveOccurred())

		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(&machine), &machine)).Should(Succeed())
		_, found := machine.Annotations[controlplanev1.PreTerminateHookCleanupAnnotation]
		Expect(found).Should(BeFalse(), "pre-terminate annotation should have been deleted")
	})
})
