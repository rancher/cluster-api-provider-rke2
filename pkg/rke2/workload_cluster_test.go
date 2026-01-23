/*
Copyright 2023 - 2026 SUSE.

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

package rke2

import (
	"context"
	"fmt"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/infrastructure"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var _ = Describe("Node metadata propagation", func() {
	var (
		err                  error
		ns                   *corev1.Namespace
		nodeName             = "node1"
		node                 *corev1.Node
		rcp                  *controlplanev1.RKE2ControlPlane
		machine              *clusterv1.Machine
		machineDifferentNode *clusterv1.Machine
		machineNodeRefStatus clusterv1.MachineStatus
		config               *bootstrapv1.RKE2Config
		clusterKey           client.ObjectKey
	)

	BeforeEach(func() {
		ns, err = testEnv.CreateNamespace(ctx, "ns")
		Expect(err).ToNot(HaveOccurred())
		clusterKey = types.NamespacedName{Namespace: ns.Name, Name: "cluster"}

		annotations := map[string]string{
			"test": "true",
		}
		node = &corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "true",
			},
			Annotations: map[string]string{
				clusterv1.MachineAnnotation: nodeName,
			},
		}}

		rcp = &controlplanev1.RKE2ControlPlane{
			Spec: controlplanev1.RKE2ControlPlaneSpec{
				Version: "v1.34.2+rke2r1",
			},
		}

		config = &bootstrapv1.RKE2Config{ObjectMeta: metav1.ObjectMeta{
			Name:      "config",
			Namespace: ns.Name,
		}, Spec: bootstrapv1.RKE2ConfigSpec{
			AgentConfig: bootstrapv1.RKE2AgentConfig{
				NodeAnnotations: annotations,
			},
		}}

		machine = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nodeName,
				Namespace: ns.Name,
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: clusterKey.Name,
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "RKE2Config",
						APIGroup: bootstrapv1.GroupVersion.Group,
						Name:     config.Name,
					},
				},
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					Kind:     infrastructure.FakeMachineKind,
					APIGroup: infrastructure.GroupVersion.Group,
					Name:     "mock-machine",
				},
			},
		}

		machineDifferentNode = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-machine",
				Namespace: ns.Name,
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: clusterKey.Name,
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "RKE2Config",
						APIGroup: bootstrapv1.GroupVersion.Group,
						Name:     config.Name,
					},
				},
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					Kind:     infrastructure.FakeMachineKind,
					APIGroup: infrastructure.GroupVersion.Group,
					Name:     "other-mock-machine",
				},
			},
		}

		machineNodeRefStatus = clusterv1.MachineStatus{
			NodeRef: clusterv1.MachineNodeReference{
				Name: nodeName,
			},
		}
	})

	AfterEach(func() {
		testEnv.Cleanup(ctx, node, ns)
	})

	It("should report the missing node", func() {
		Expect(testEnv.Create(ctx, config)).To(Succeed())
		Expect(testEnv.Create(ctx, machine)).To(Succeed())

		machines := collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machine,
		}})

		m := &Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		w, err := m.NewWorkload(ctx, testEnv.GetClient(), testEnv.GetConfig(), clusterKey)
		Expect(err).ToNot(HaveOccurred())

		cp, err := NewControlPlane(ctx, m, testEnv.GetClient(), nil, rcp, machines)
		Expect(err).ToNot(HaveOccurred())
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.Nodes).To(HaveLen(0))
		Expect(cp.Rke2Configs).To(HaveLen(1))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(HaveField(
			"Status", Equal(metav1.ConditionUnknown),
		))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(HaveField(
			"Message", MatchRegexp("node .+ not found"),
		))
	})

	It("should report the missing config", func() {
		Expect(testEnv.Create(ctx, node)).To(Succeed())
		Expect(testEnv.Create(ctx, machine)).To(Succeed())

		machines := collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machine,
		}})

		m := &Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		w, err := m.NewWorkload(ctx, testEnv.GetClient(), testEnv.GetConfig(), clusterKey)
		Expect(err).ToNot(HaveOccurred())
		cp, err := NewControlPlane(ctx, m, testEnv.GetClient(), nil, rcp, machines)
		Expect(err).ToNot(HaveOccurred())
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.Nodes).To(HaveLen(1))
		Expect(cp.Rke2Configs).To(HaveLen(0))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(HaveField(
			"Status", Equal(metav1.ConditionUnknown),
		))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(HaveField(
			"Message", MatchRegexp("node .+ RKE2Config not found"),
		))
	})

	It("should recover from error conditions on successfull node patch", func() {
		Expect(testEnv.Create(ctx, config)).To(Succeed())
		Expect(testEnv.Create(ctx, machine)).To(Succeed())

		machines := collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machine,
		}})

		m := &Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		w, err := m.NewWorkload(ctx, testEnv.GetClient(), testEnv.GetConfig(), clusterKey)
		Expect(err).ToNot(HaveOccurred())
		cp, err := NewControlPlane(ctx, m, testEnv.GetClient(), nil, rcp, machines)
		Expect(err).ToNot(HaveOccurred())
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.Nodes).To(HaveLen(0))
		Expect(cp.Rke2Configs).To(HaveLen(1))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(HaveField(
			"Status", Equal(metav1.ConditionUnknown),
		))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(HaveField(
			"Message", MatchRegexp("node .+ not found"),
		))

		Expect(testEnv.Create(ctx, node)).To(Succeed())
		Eventually(ctx, func() map[string]*corev1.Node {
			Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
			return w.Nodes
		}).Should(HaveLen(1))
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(cp.Rke2Configs).To(HaveLen(1))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(HaveField(
			"Status", Equal(metav1.ConditionTrue),
		))
		Expect(w.Nodes[nodeName].GetAnnotations()).To(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: nodeName,
		}))
	})

	It("should set the node annotations", func() {
		Expect(testEnv.Create(ctx, node)).To(Succeed())
		Expect(testEnv.Create(ctx, config)).To(Succeed())
		Expect(testEnv.Create(ctx, machine)).To(Succeed())

		machines := collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machine,
		}})

		m := &Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		w, err := m.NewWorkload(ctx, testEnv.GetClient(), testEnv.GetConfig(), clusterKey)
		Expect(err).ToNot(HaveOccurred())
		cp, err := NewControlPlane(ctx, m, testEnv.GetClient(), nil, rcp, machines)
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(err).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.Nodes).To(HaveLen(1))
		Expect(cp.Rke2Configs).To(HaveLen(1))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(HaveField(
			"Status", Equal(metav1.ConditionTrue),
		))
		Expect(w.Nodes[nodeName].GetAnnotations()).To(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: nodeName,
		}))

		result := &corev1.Node{}
		Eventually(ctx, func() map[string]string {
			Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(node), result)).To(Succeed())
			return result.GetAnnotations()
		}).WithTimeout(10 * time.Second).Should(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: nodeName,
		}))
	})

	It("should set the node annotations for an arbitrary node reference", func() {
		node.SetAnnotations(map[string]string{
			clusterv1.MachineAnnotation: machineDifferentNode.Name,
		})
		Expect(testEnv.Create(ctx, node)).To(Succeed())
		Expect(testEnv.Create(ctx, config)).To(Succeed())
		Expect(testEnv.Create(ctx, machineDifferentNode)).To(Succeed())
		machineDifferentNode.Status = machineNodeRefStatus
		Expect(testEnv.Status().Update(ctx, machineDifferentNode)).To(Succeed())

		machines := collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machineDifferentNode,
		}})

		m := &Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		w, err := m.NewWorkload(ctx, testEnv.GetClient(), testEnv.GetConfig(), clusterKey)
		Expect(err).ToNot(HaveOccurred())
		cp, err := NewControlPlane(ctx, m, testEnv.GetClient(), nil, rcp, machines)
		Expect(err).ToNot(HaveOccurred())
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.Nodes).To(HaveLen(1))
		Expect(cp.Rke2Configs).To(HaveLen(1))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machineDifferentNode.Name], controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(HaveField(
			"Status", Equal(metav1.ConditionTrue),
		))
		Expect(w.Nodes[nodeName].GetAnnotations()).To(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: machineDifferentNode.Name,
		}))

		Eventually(func() error {
			result := &corev1.Node{}
			if err := testEnv.Get(ctx, client.ObjectKeyFromObject(node), result); err != nil {
				return err
			}
			if !reflect.DeepEqual(result.GetAnnotations(), map[string]string{
				"test":                      "true",
				clusterv1.MachineAnnotation: machineDifferentNode.Name,
			}) {
				return fmt.Errorf("annotations do not match: got %v", result.GetAnnotations())
			}
			return nil
		}, 5*time.Second).Should(Succeed())
	})

	It("should recover from error condition on successfull node patch for arbitrary node name", func() {
		node.SetAnnotations(map[string]string{
			clusterv1.MachineAnnotation: machineDifferentNode.Name,
		})
		Expect(testEnv.Create(ctx, node)).To(Succeed())
		Expect(testEnv.Create(ctx, config)).To(Succeed())
		Expect(testEnv.Create(ctx, machineDifferentNode)).To(Succeed())

		machines := collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machineDifferentNode,
		}})

		m := &Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		w, err := m.NewWorkload(ctx, testEnv.GetClient(), testEnv.GetConfig(), clusterKey)
		Expect(err).ToNot(HaveOccurred())
		cp, err := NewControlPlane(ctx, m, testEnv.GetClient(), nil, rcp, machines)
		Expect(err).ToNot(HaveOccurred())
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.Nodes).To(HaveLen(1))
		Expect(cp.Rke2Configs).To(HaveLen(1))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machineDifferentNode.Name], controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(HaveField(
			"Status", Equal(metav1.ConditionUnknown),
		))
		Expect(conditions.Get(cp.Machines[machineDifferentNode.Name], controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(HaveField(
			"Message", MatchRegexp("node .+ not found"),
		))

		machineDifferentNode.Status = machineNodeRefStatus
		Expect(testEnv.Status().Update(ctx, machineDifferentNode)).To(Succeed())

		machines = collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machineDifferentNode,
		}})
		cp, err = NewControlPlane(ctx, m, testEnv.GetClient(), nil, rcp, machines)
		Expect(err).ToNot(HaveOccurred())
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())

		Expect(conditions.Get(cp.Machines[machineDifferentNode.Name], controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition)).To(HaveField(
			"Status", Equal(metav1.ConditionTrue),
		))
		Expect(w.Nodes[nodeName].GetAnnotations()).To(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: machineDifferentNode.Name,
		}))

		result := &corev1.Node{}
		Eventually(func() map[string]string {
			Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(node), result)).To(Succeed())
			return result.GetAnnotations()
		}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: machineDifferentNode.Name,
		}))
	})
})

var _ = Describe("Cloud-init fields validation", func() {
	var (
		err error
		ns  *corev1.Namespace
	)

	BeforeEach(func() {
		ns, err = testEnv.CreateNamespace(ctx, "ns")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		testEnv.Cleanup(ctx, ns)
	})

	It("should prevent populating config and data fields", func() {
		Expect(testEnv.Create(ctx, &bootstrapv1.RKE2Config{ObjectMeta: metav1.ObjectMeta{
			Name:      "config",
			Namespace: ns.Name,
		}, Spec: bootstrapv1.RKE2ConfigSpec{
			AgentConfig: bootstrapv1.RKE2AgentConfig{
				AdditionalUserData: bootstrapv1.AdditionalUserData{
					Config: "some",
					Data: map[string]string{
						"no": "way",
					},
				},
			},
		}})).ToNot(Succeed())
	})
})

var _ = Describe("ClusterStatus validation", func() {
	var (
		err           error
		ns            *corev1.Namespace
		node1         *corev1.Node
		node2         *corev1.Node
		servingSecret *corev1.Secret
		connectionErr interceptor.Funcs
	)

	BeforeEach(func() {
		ns, err = testEnv.CreateNamespace(ctx, "ns")
		Expect(err).ToNot(HaveOccurred())

		node1 = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "true",
				},
				Annotations: map[string]string{
					clusterv1.MachineAnnotation: "node1",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				}},
			},
		}

		node2 = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node2",
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "true",
				},
				Annotations: map[string]string{
					clusterv1.MachineAnnotation: "node2",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				}},
			},
		}

		servingSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rke2ServingSecretKey,
				Namespace: metav1.NamespaceSystem,
			},
		}

		connectionErr = interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
				return errors.New("test connection error")
			},
		}
	})

	AfterEach(func() {
		testEnv.Cleanup(ctx, ns)
	})

	It("should set HasRKE2ServingSecret if servingSecret found", func() {
		fakeClient := fake.NewClientBuilder().WithObjects([]client.Object{servingSecret}...).Build()
		w := &Workload{
			Client: fakeClient,
			Nodes: map[string]*corev1.Node{
				node1.Name: node1,
				node2.Name: node2,
			},
		}
		status := w.ClusterStatus(ctx)
		Expect(status.Nodes).To(BeEquivalentTo(2), "There are 2 nodes in this cluster")
		Expect(status.ReadyNodes).To(BeEquivalentTo(1), "Only 1 node has the NodeReady condition")
		Expect(status.HasRKE2ServingSecret).To(BeTrue(), "rke2-serving Secret exists in kube-system namespace")
	})
	It("should not set HasRKE2ServingSecret if servingSecret not existing", func() {
		fakeClient := fake.NewClientBuilder().Build()
		w := &Workload{
			Client: fakeClient,
			Nodes: map[string]*corev1.Node{
				node1.Name: node1,
				node2.Name: node2,
			},
		}
		status := w.ClusterStatus(ctx)
		Expect(status.Nodes).To(BeEquivalentTo(2), "There are 2 nodes in this cluster")
		Expect(status.ReadyNodes).To(BeEquivalentTo(1), "Only 1 node has the NodeReady condition")
		Expect(status.HasRKE2ServingSecret).To(BeFalse(), "rke2-serving Secret does not exists in kube-system namespace")
	})
	It("should not set HasRKE2ServingSecret if client returns error", func() {
		fakeClient := fake.NewClientBuilder().WithInterceptorFuncs(connectionErr).Build()
		w := &Workload{
			Client: fakeClient,
			Nodes: map[string]*corev1.Node{
				node1.Name: node1,
				node2.Name: node2,
			},
		}
		status := w.ClusterStatus(ctx)
		Expect(status.Nodes).To(BeEquivalentTo(2), "There are 2 nodes in this cluster")
		Expect(status.ReadyNodes).To(BeEquivalentTo(1), "Only 1 node has the NodeReady condition")
		Expect(status.HasRKE2ServingSecret).To(BeFalse(), "On connection error assume control plane not initialized")
	})
})
