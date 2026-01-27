//go:build e2e
// +build e2e

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

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/rke2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("In-place propagation", func() {
	var (
		specName            = "in-place-propagation"
		namespace           *corev1.Namespace
		cancelWatches       context.CancelFunc
		result              *ApplyClusterTemplateAndWaitResult
		clusterName         string
		clusterctlLogFolder string
	)

	BeforeEach(func() {
		Expect(e2eConfig).ToNot(BeNil(), "Invalid argument. e2eConfig can't be nil when calling %s spec", specName)
		Expect(clusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. clusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(bootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. bootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(artifactFolder, 0o755)).To(Succeed(), "Invalid argument. artifactFolder can't be created for %s spec", specName)

		Expect(e2eConfig.Variables).To(HaveKey(KubernetesVersion))

		By("Initializing the bootstrap cluster")
		initBootstrapCluster(bootstrapClusterProxy, e2eConfig, clusterctlConfigPath, artifactFolder)

		clusterName = fmt.Sprintf("caprke2-e2e-%s", util.RandomString(6))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, bootstrapClusterProxy, artifactFolder)

		result = new(ApplyClusterTemplateAndWaitResult)

		// We need to override clusterctl apply log folder to avoid getting our credentials exposed.
		clusterctlLogFolder = filepath.Join(os.TempDir(), "clusters", bootstrapClusterProxy.GetName())
	})

	AfterEach(func() {
		err := CollectArtifacts(ctx, bootstrapClusterProxy.GetKubeconfigPath(), filepath.Join(artifactFolder, bootstrapClusterProxy.GetName(), clusterName+specName))
		Expect(err).ToNot(HaveOccurred())

		cleanInput := cleanupInput{
			SpecName:             specName,
			Cluster:              result.Cluster,
			KubeconfigPath:       result.KubeconfigPath,
			ClusterProxy:         bootstrapClusterProxy,
			Namespace:            namespace,
			CancelWatches:        cancelWatches,
			IntervalsGetter:      e2eConfig.GetIntervals,
			SkipCleanup:          skipCleanup,
			ArtifactFolder:       artifactFolder,
			AdditionalCleanup:    cleanupInstallation(ctx, clusterctlLogFolder, clusterctlConfigPath, bootstrapClusterProxy),
			ClusterctlConfigPath: clusterctlConfigPath,
		}

		dumpSpecResourcesAndCleanup(ctx, cleanInput)
	})

	Context("Creating a single control-plane cluster", func() {
		It("Should create a cluster with 3 control plane nodes and perform in-place-propagation tests", func() {
			By("Initializes with 3 control plane nodes")
			ApplyClusterTemplateAndWait(ctx, ApplyClusterTemplateAndWaitInput{
				ClusterProxy: bootstrapClusterProxy,
				ConfigCluster: clusterctl.ConfigClusterInput{
					LogFolder:                clusterctlLogFolder,
					ClusterctlConfigPath:     clusterctlConfigPath,
					KubeconfigPath:           bootstrapClusterProxy.GetKubeconfigPath(),
					InfrastructureProvider:   "docker",
					Flavor:                   "docker",
					Namespace:                namespace.Name,
					ClusterName:              clusterName,
					KubernetesVersion:        e2eConfig.MustGetVariable(KubernetesVersion),
					ControlPlaneMachineCount: ptr.To(int64(3)),
					WorkerMachineCount:       ptr.To(int64(0)),
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			WaitForControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       bootstrapClusterProxy.GetClient(),
				ControlPlane: client.ObjectKeyFromObject(result.ControlPlane),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			By("Verifying the cluster is available")
			framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
				Getter: bootstrapClusterProxy.GetClient(),  
				Name:      result.Cluster.Name,
				Namespace: result.Cluster.Namespace,
			})

			By("Fetching all Machines")
			machineList := GetMachinesByCluster(ctx, GetMachinesByClusterInput{
				Lister:      bootstrapClusterProxy.GetClient(),
				ClusterName: result.Cluster.Name,
				Namespace:   result.Cluster.Namespace,
			})
			Expect(machineList.Items).ShouldNot(BeEmpty(), "There must be at least one Machine")

			By("Fetch RKE2 control plane")
			rke2ControlPlane := GetRKE2ControlPlaneByCluster(ctx, GetRKE2ControlPlaneByClusterInput{
				Lister:      bootstrapClusterProxy.GetClient(),
				ClusterName: result.Cluster.Name,
				Namespace:   result.Cluster.Namespace,
			})
			Expect(rke2ControlPlane).ToNot(BeNil(), "There must be a RKE2 control plane")
			Expect(rke2ControlPlane.Spec.MachineTemplate).ToNot(BeNil(), "MachineTemplate must not be nil")
			Expect(rke2ControlPlane.Spec.MachineTemplate.ObjectMeta).ToNot(BeNil(), "ObjectMeta in MachineTemplate must not be nil")

			rke2ControlPlaneOriginal := rke2ControlPlane.DeepCopy()

			// Ensure labels and annotations maps are initialized
			if rke2ControlPlane.Spec.MachineTemplate.ObjectMeta.Labels == nil {
				rke2ControlPlane.Spec.MachineTemplate.ObjectMeta.Labels = make(map[string]string)
			}
			if rke2ControlPlane.Spec.MachineTemplate.ObjectMeta.Annotations == nil {
				rke2ControlPlane.Spec.MachineTemplate.ObjectMeta.Annotations = make(map[string]string)
			}

			By("Setting new labels, annotations, and timeouts")
			// Set new labels and annotations
			rke2ControlPlane.Spec.MachineTemplate.ObjectMeta.Labels["test-label"] = "test-label-value"
			rke2ControlPlane.Spec.MachineTemplate.ObjectMeta.Annotations["test-annotation"] = "test-annotation-value"

			// Set new timeouts for NodeDrainTimeoutSeconds, NodeDeletionTimeoutSeconds and NodeVolumeDetachTimeoutSeconds.
			rke2ControlPlane.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds = ptr.To(int32(timeout240s))
			rke2ControlPlane.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds = ptr.To(int32(timeout240s))
			rke2ControlPlane.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = ptr.To(int32(timeout480s))

			// Patch the RKE2 control plane
			By("Patching RKE2 control plane with new labels, annotations, and timeouts")
			Expect(bootstrapClusterProxy.GetClient().Patch(ctx, rke2ControlPlane, client.MergeFrom(rke2ControlPlaneOriginal))).To(Succeed(), "Failed to patch the RKE2 control plane")

			// Ensure no Machine rollout is triggered
			EnsureNoMachineRollout(ctx, GetMachinesByClusterInput{
				Lister:      bootstrapClusterProxy.GetClient(),
				ClusterName: result.Cluster.Name,
				Namespace:   result.Cluster.Namespace,
			}, machineList)

			// Check NodeDrainTimeoutSeconds, NodeDeletionTimeoutSeconds and NodeVolumeDetachTimeoutSeconds values are propagated to Machines
			By("Check NodeDrainTimeoutSeconds, NodeDeletionTimeoutSeconds and NodeVolumeDetachTimeoutSeconds values are propagated to Machines")
			Eventually(func() error {
				By("Fetching all Machines")
				machineList := GetMachinesByCluster(ctx, GetMachinesByClusterInput{
					Lister:      bootstrapClusterProxy.GetClient(),
					ClusterName: result.Cluster.Name,
					Namespace:   result.Cluster.Namespace,
				})
				Expect(machineList.Items).ShouldNot(BeEmpty(), "There must be at least one Machine")
				for _, machine := range machineList.Items {
					if machine.Spec.Deletion.NodeDrainTimeoutSeconds != nil && *machine.Spec.Deletion.NodeDrainTimeoutSeconds != timeout240s {
						return fmt.Errorf("NodeDrainTimeoutSeconds value is not propagated to Machine %s/%s", machine.Namespace, machine.Name)
					}
					if machine.Spec.Deletion.NodeDeletionTimeoutSeconds != nil && *machine.Spec.Deletion.NodeDeletionTimeoutSeconds != timeout240s {
						return fmt.Errorf("NodeDeletionTimeoutSeconds value is not propagated to Machine %s/%s", machine.Namespace, machine.Name)
					}
					if machine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds != nil && *machine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds != timeout480s {
						return fmt.Errorf("NodeVolumeDetachTimeoutSeconds value is not propagated to Machine %s/%s", machine.Namespace, machine.Name)
					}
				}
				return nil
			}, 10*time.Minute, 15*time.Second).Should(Succeed(), "Node timeouts are not propagated to Machines")

			// Check labels/annotations are propagated to Machines and associated InfraMachines/RKE2Configs.
			By("Verifying labels and annotations are propagated to Machines, InfraMachines, and RKE2Configs")
			expectedLabelKey := "test-label"
			expectedLabelValue := "test-label-value"
			expectedAnnotationKey := "test-annotation"
			expectedAnnotationValue := "test-annotation-value"

			Eventually(func() error {
				// Fetch all Machines
				machineList := GetMachinesByCluster(ctx, GetMachinesByClusterInput{
					Lister:      bootstrapClusterProxy.GetClient(),
					ClusterName: result.Cluster.Name,
					Namespace:   result.Cluster.Namespace,
				})

				machines := collections.FromMachineList(machineList)
				// Fetch InfraMachines associated with Machines
				infraMachines, err := rke2.GetInfraResources(ctx, bootstrapClusterProxy.GetClient(), machines)
				if err != nil {
					return fmt.Errorf("failed to fetch InfraMachines: %w", err)
				}

				// Fetch RKE2Configs associated with Machines
				rke2Configs, err := rke2.GetRKE2Configs(ctx, bootstrapClusterProxy.GetClient(), machines)
				if err != nil {
					return fmt.Errorf("failed to fetch RKE2Configs: %w", err)
				}

				for _, machine := range machineList.Items {
					// Check labels and annotations on the Machine
					if machine.Labels[expectedLabelKey] != expectedLabelValue {
						return fmt.Errorf("label %s not propagated to Machine %s/%s", expectedLabelKey, machine.Namespace, machine.Name)
					}
					if machine.Annotations[expectedAnnotationKey] != expectedAnnotationValue {
						return fmt.Errorf("annotation %s not propagated to Machine %s/%s", expectedAnnotationKey, machine.Namespace, machine.Name)
					}

					// Find the associated InfraMachine
					infraMachine, infraMachineFound := infraMachines[machine.Name]
					if !infraMachineFound {
						return fmt.Errorf("InfraMachine not found for Machine %s/%s", machine.Namespace, machine.Name)
					}

					// Ensure InfraMachine is associated with the Machine
					if !isOwnedBy(infraMachine, &machine) {
						return fmt.Errorf("InfraMachine %s/%s is not owned by Machine %s/%s", infraMachine.GetNamespace(), infraMachine.GetName(), machine.Namespace, machine.Name)
					}

					if infraMachine.GetLabels()[expectedLabelKey] != expectedLabelValue {
						return fmt.Errorf("label %s not propagated to InfraMachine %s/%s", expectedLabelKey, infraMachine.GetNamespace(), infraMachine.GetName())
					}
					if infraMachine.GetAnnotations()[expectedAnnotationKey] != expectedAnnotationValue {
						return fmt.Errorf("annotation %s not propagated to InfraMachine %s/%s", expectedAnnotationKey, infraMachine.GetNamespace(), infraMachine.GetName())
					}

					// Find the associated RKE2Config
					rke2Config, rke2ConfigFound := rke2Configs[machine.Name]
					if !rke2ConfigFound {
						return fmt.Errorf("RKE2Config not found for Machine %s/%s", machine.Namespace, machine.Name)
					}

					// Ensure RKE2Config is associated with the Machine
					if !isOwnedBy(rke2Config, &machine) {
						return fmt.Errorf("RKE2Config %s/%s is not owned by Machine %s/%s", rke2Config.GetNamespace(), rke2Config.GetName(), machine.Namespace, machine.Name)
					}
					if rke2Config.Labels[expectedLabelKey] != expectedLabelValue {
						return fmt.Errorf("label %s not propagated to RKE2Config %s/%s", expectedLabelKey, rke2Config.GetNamespace(), rke2Config.GetName())
					}
					if rke2Config.Annotations[expectedAnnotationKey] != expectedAnnotationValue {
						return fmt.Errorf("annotation %s not propagated to RKE2Config %s/%s", expectedAnnotationKey, rke2Config.GetNamespace(), rke2Config.GetName())
					}

				}
				return nil
			}, 5*time.Minute, 10*time.Second).Should(Succeed(), "Labels/annotations not propagated or associations not correct")

			By("Waiting for machines to have propagated metadata")
			for _, machine := range machineList.Items {
				machine := machine

				WaitForMachineConditions(ctx, WaitForMachineConditionsInput{
					Getter:    bootstrapClusterProxy.GetClient(),
					Machine:   &machine,
					Checker:   conditions.IsTrue,
					Condition: controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition,
				}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)
			}

			By("Verifying annotations have been propagated to nodes")
			downstreamProxy := framework.NewClusterProxy("metadata", result.KubeconfigPath, initScheme())
			Expect(downstreamProxy).ToNot(BeNil(), "Failed to get a metadata cluster proxy")

			nodeList := &corev1.NodeList{}
			Expect(downstreamProxy.GetClient().List(ctx, nodeList)).Should(Succeed())
			for _, node := range nodeList.Items {
				value, found := node.GetObjectMeta().GetAnnotations()["test"]
				Expect(found).Should(BeTrue(), "'test' annotation must be found on node")
				Expect(value).Should(Equal("true"), "'test' node annotation should have 'true' value")
			}
		})
	})
})

// isOwnedBy checks if the object is owned by the specified owner.
func isOwnedBy(obj metav1.Object, owner metav1.Object) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == owner.GetUID() {
			return true
		}
	}
	return false
}
