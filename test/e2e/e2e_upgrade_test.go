//go:build e2e
// +build e2e

/*
Copyright 2024 SUSE.

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

	"github.com/drone/envsubst/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Provider upgrade", func() {
	var (
		specName            = "provider-upgrade"
		namespace           *corev1.Namespace
		cancelWatches       context.CancelFunc
		clusterName         string
		clusterctlLogFolder string
	)

	BeforeEach(func() {
		Expect(e2eConfig).ToNot(BeNil(), "Invalid argument. e2eConfig can't be nil when calling %s spec", specName)
		Expect(clusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. clusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(bootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. bootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(artifactFolder, 0o755)).To(Succeed(), "Invalid argument. artifactFolder can't be created for %s spec", specName)

		Expect(e2eConfig.Variables).To(HaveKey(KubernetesVersion))

		clusterName = fmt.Sprintf("caprke2-e2e-%s-upgrade", util.RandomString(6))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, bootstrapClusterProxy, artifactFolder)

		// We need to override clusterctl apply log folder to avoid getting our credentials exposed.
		clusterctlLogFolder = filepath.Join(os.TempDir(), "clusters", bootstrapClusterProxy.GetName())
	})

	AfterEach(func() {
		err := CollectArtifacts(ctx, bootstrapClusterProxy.GetKubeconfigPath(), filepath.Join(artifactFolder, bootstrapClusterProxy.GetName(), clusterName+specName))
		Expect(err).ToNot(HaveOccurred())

		cluster := &clusterv1.Cluster{}
		clusterKey := types.NamespacedName{Name: clusterName, Namespace: namespace.Name}
		Expect(bootstrapClusterProxy.GetClient().Get(ctx, clusterKey, cluster)).Should(Succeed())
		kubeConfigPath := WriteKubeconfigSecretToDisk(ctx, bootstrapClusterProxy.GetClient(), clusterKey.Namespace, clusterKey.Name+"-kubeconfig")

		cleanInput := cleanupInput{
			SpecName:             specName,
			Cluster:              cluster,
			KubeconfigPath:       kubeConfigPath,
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
		It("Should create a cluster with v0.21.1 and perform upgrade to latest version", func() {
			By("Installing v0.21.1 bootstrap/controlplane provider version")
			initUpgradableBootstrapCluster(bootstrapClusterProxy, e2eConfig, clusterctlConfigPath, artifactFolder)

			clusterTemplate, err := envsubst.Eval(string(ClusterTemplateDockerV1Beta1), func(s string) string {
				switch s {
				case "CLUSTER_NAME":
					return clusterName
				case "CONTROL_PLANE_MACHINE_COUNT":
					return "3"
				case "WORKER_MACHINE_COUNT":
					return "1"
				default:
					return e2eConfig.MustGetVariable(s)
				}
			})
			Expect(err).ToNot(HaveOccurred())

			Byf("Applying the cluster template yaml of cluster %s", klog.KRef(namespace.Name, clusterName))
			Eventually(func() error {
				return Apply(ctx, bootstrapClusterProxy, []byte(clusterTemplate), "--namespace", namespace.Name)
			}, e2eConfig.GetIntervals(specName, "wait-cluster")...).Should(Succeed(), "Failed to apply the cluster template")

			WaitForClusterReadyV1Beta1(ctx, WaitForClusterReadyInput{
				Getter:    bootstrapClusterProxy.GetClient(),
				Name:      clusterName,
				Namespace: namespace.Name,
			}, e2eConfig.GetIntervals(specName, "wait-cluster")...)

			By("Fetching all Machines")
			machineNames := GetMachineNamesByClusterv1Beta1(ctx, GetMachinesByClusterInput{
				Lister:      bootstrapClusterProxy.GetClient(),
				ClusterName: clusterName,
				Namespace:   namespace.Name,
			})
			Expect(machineNames).ShouldNot(BeEmpty(), "There must be at least one Machine")

			By("Upgrading to next bootstrap/controlplane provider version")
			UpgradeManagementCluster(ctx, clusterctl.UpgradeManagementClusterAndWaitInput{
				ClusterProxy:            bootstrapClusterProxy,
				ClusterctlConfigPath:    clusterctlConfigPath,
				InfrastructureProviders: []string{"docker:v1.11.5"},
				CoreProvider:            "cluster-api:v1.11.5",
				BootstrapProviders:      []string{"rke2-bootstrap:v0.22.99"},
				ControlPlaneProviders:   []string{"rke2-control-plane:v0.22.99"},
				LogFolder:               clusterctlLogFolder,
			})

			By("Verifying the cluster is available")
			framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
				Getter:    bootstrapClusterProxy.GetClient(),
				Name:      clusterName,
				Namespace: namespace.Name,
			})

			EnsureNoMachineRollout(ctx, GetMachinesByClusterInput{
				Lister:      bootstrapClusterProxy.GetClient(),
				ClusterName: clusterName,
				Namespace:   namespace.Name,
			}, machineNames)

			By("Scaling down control plane to 2")
			controlPlane := &controlplanev1.RKE2ControlPlane{}
			controlPlanes := &controlplanev1.RKE2ControlPlaneList{}
			Expect(bootstrapClusterProxy.GetClient().List(ctx, controlPlanes,
				client.MatchingLabels{clusterv1.ClusterNameLabel: clusterName},
				client.InNamespace(namespace.Name))).Should(Succeed())
			Expect(len(controlPlanes.Items)).Should(Equal(1), "Only 1 RKE2ControlPlane is expected")
			controlPlaneKey := client.ObjectKeyFromObject(&controlPlanes.Items[0])

			Eventually(func() error {
				Expect(bootstrapClusterProxy.GetClient().Get(ctx, controlPlaneKey, controlPlane)).Should(Succeed())
				controlPlane.Spec.Replicas = ptr.To(int32(2))
				return bootstrapClusterProxy.GetClient().Update(ctx, controlPlane)
			}).WithPolling(10 * time.Second).WithTimeout(2 * time.Minute).Should(Succeed())

			By("Scaling up workers to 2")
			machineDeployment := &clusterv1.MachineDeployment{}
			machineDeployments := &clusterv1.MachineDeploymentList{}
			Expect(bootstrapClusterProxy.GetClient().List(ctx, machineDeployments,
				client.MatchingLabels{clusterv1.ClusterNameLabel: clusterName},
				client.InNamespace(namespace.Name))).Should(Succeed())
			Expect(len(machineDeployments.Items)).Should(Equal(1), "Only 1 MachineDeployment is expected")
			machineDeploymentKey := client.ObjectKeyFromObject(&machineDeployments.Items[0])

			Eventually(func() error {
				Expect(bootstrapClusterProxy.GetClient().Get(ctx, machineDeploymentKey, machineDeployment)).Should(Succeed())
				machineDeployment.Spec.Replicas = ptr.To(int32(2))
				return bootstrapClusterProxy.GetClient().Update(ctx, machineDeployment)
			}).WithPolling(10 * time.Second).WithTimeout(2 * time.Minute).Should(Succeed())

			WaitForControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       bootstrapClusterProxy.GetClient(),
				ControlPlane: controlPlaneKey,
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			By("Verifying the cluster is available")
			framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
				Getter:    bootstrapClusterProxy.GetClient(),
				Name:      clusterName,
				Namespace: namespace.Name,
			})

			By("Scale down control plane and workers to 1 with kubernetes version upgrade")
			versionUpgradeTo := e2eConfig.MustGetVariable(KubernetesVersionUpgradeTo)

			Eventually(func() error {
				Expect(bootstrapClusterProxy.GetClient().Get(ctx, controlPlaneKey, controlPlane)).Should(Succeed())
				controlPlane.Spec.Replicas = ptr.To(int32(1))
				controlPlane.Spec.Version = versionUpgradeTo + "+rke2r1"
				return bootstrapClusterProxy.GetClient().Update(ctx, controlPlane)
			}).WithPolling(10 * time.Second).WithTimeout(2 * time.Minute).Should(Succeed())

			Eventually(func() error {
				Expect(bootstrapClusterProxy.GetClient().Get(ctx, machineDeploymentKey, machineDeployment)).Should(Succeed())
				machineDeployment.Spec.Replicas = ptr.To(int32(1))
				machineDeployment.Spec.Template.Spec.Version = versionUpgradeTo + "+rke2r1"
				return bootstrapClusterProxy.GetClient().Update(ctx, machineDeployment)
			}).WithPolling(10 * time.Second).WithTimeout(2 * time.Minute).Should(Succeed())

			WaitForClusterToUpgrade(ctx, WaitForClusterToUpgradeInput{
				Reader:              bootstrapClusterProxy.GetClient(),
				ControlPlane:        controlPlane,
				MachineDeployments:  []*clusterv1.MachineDeployment{machineDeployment},
				VersionAfterUpgrade: e2eConfig.MustGetVariable(KubernetesVersionUpgradeTo),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			WaitForControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       bootstrapClusterProxy.GetClient(),
				ControlPlane: client.ObjectKeyFromObject(controlPlane),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			By("Verifying the cluster is available")
			framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
				Getter:    bootstrapClusterProxy.GetClient(),
				Name:      clusterName,
				Namespace: namespace.Name,
			})
		})
	})
})
