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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Workload cluster creation", func() {
	var (
		specName            = "create-workload-cluster"
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
		Expect(os.MkdirAll(artifactFolder, 0755)).To(Succeed(), "Invalid argument. artifactFolder can't be created for %s spec", specName)

		Expect(e2eConfig.Variables).To(HaveKey(KubernetesVersion))

		clusterName = fmt.Sprintf("caprke2-e2e-%s-upgrade", util.RandomString(6))

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
			SpecName:          specName,
			Cluster:           result.Cluster,
			ClusterProxy:      bootstrapClusterProxy,
			Namespace:         namespace,
			CancelWatches:     cancelWatches,
			IntervalsGetter:   e2eConfig.GetIntervals,
			SkipCleanup:       skipCleanup,
			ArtifactFolder:    artifactFolder,
			AdditionalCleanup: cleanupInstallation(ctx, clusterctlLogFolder, clusterctlConfigPath, bootstrapClusterProxy),
		}

		dumpSpecResourcesAndCleanup(ctx, cleanInput)
	})

	Context("Creating a single control-plane cluster", func() {
		It("Should create a cluster with v0.16.0 and perform upgrade to latest version", func() {
			By("Installing v0.16.0 boostrap/controlplane provider version")
			initUpgradableBootstrapCluster(bootstrapClusterProxy, e2eConfig, clusterctlConfigPath, artifactFolder)

			By("Initializing the cluster")
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
					WorkerMachineCount:       ptr.To(int64(1)),
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
				SkipPreviousVersionChecks:    true,
			}, result)

			WaitForControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       bootstrapClusterProxy.GetClient(),
				ControlPlane: client.ObjectKeyFromObject(result.ControlPlane),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			WaitForClusterReady(ctx, WaitForClusterReadyInput{
				Getter:    bootstrapClusterProxy.GetClient(),
				Name:      result.Cluster.Name,
				Namespace: result.Cluster.Namespace,
			}, e2eConfig.GetIntervals(specName, "wait-cluster")...)

			By("Fetching all Machines")
			machineList := GetMachinesByCluster(ctx, GetMachinesByClusterInput{
				Lister:      bootstrapClusterProxy.GetClient(),
				ClusterName: result.Cluster.Name,
				Namespace:   result.Cluster.Namespace,
			})
			Expect(machineList.Items).ShouldNot(BeEmpty(), "There must be at least one Machine")

			By("Upgrading to next boostrap/controlplane provider version")
			UpgradeManagementCluster(ctx, clusterctl.UpgradeManagementClusterAndWaitInput{
				ClusterProxy:          bootstrapClusterProxy,
				ClusterctlConfigPath:  clusterctlConfigPath,
				BootstrapProviders:    []string{"rke2-bootstrap:v0.17.99"},
				ControlPlaneProviders: []string{"rke2-control-plane:v0.17.99"},
				LogFolder:             clusterctlLogFolder,
			})

			WaitForControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       bootstrapClusterProxy.GetClient(),
				ControlPlane: client.ObjectKeyFromObject(result.ControlPlane),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			WaitForClusterReady(ctx, WaitForClusterReadyInput{
				Getter:    bootstrapClusterProxy.GetClient(),
				Name:      result.Cluster.Name,
				Namespace: result.Cluster.Namespace,
			}, e2eConfig.GetIntervals(specName, "wait-cluster")...)

			EnsureNoMachineRollout(ctx, GetMachinesByClusterInput{
				Lister:      bootstrapClusterProxy.GetClient(),
				ClusterName: result.Cluster.Name,
				Namespace:   result.Cluster.Namespace,
			}, machineList)

			By("Scaling down control plane to 2 and workers up to 2")
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
					ControlPlaneMachineCount: ptr.To(int64(2)),
					WorkerMachineCount:       ptr.To(int64(2)),
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			WaitForControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       bootstrapClusterProxy.GetClient(),
				ControlPlane: client.ObjectKeyFromObject(result.ControlPlane),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			By("Scale down control plane and workers to 1 with kubernetes version upgrade")
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
					KubernetesVersion:        e2eConfig.MustGetVariable(KubernetesVersionUpgradeTo),
					ControlPlaneMachineCount: ptr.To(int64(1)),
					WorkerMachineCount:       ptr.To(int64(1)),
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			WaitForClusterToUpgrade(ctx, WaitForClusterToUpgradeInput{
				Reader:              bootstrapClusterProxy.GetClient(),
				ControlPlane:        result.ControlPlane,
				MachineDeployments:  result.MachineDeployments,
				VersionAfterUpgrade: e2eConfig.MustGetVariable(KubernetesVersionUpgradeTo),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			WaitForControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       bootstrapClusterProxy.GetClient(),
				ControlPlane: client.ObjectKeyFromObject(result.ControlPlane),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)
		})
	})
})
