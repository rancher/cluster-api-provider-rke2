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

	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Bootstrap & Pivot", func() {
	var (
		specName            = "bootstrap-pivot-cluster"
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

		By("Initializing the bootstrap cluster")
		initBootstrapCluster(bootstrapClusterProxy, e2eConfig, clusterctlConfigPath, artifactFolder)

		clusterName = fmt.Sprintf("caprke2-e2e-%s-pivot", util.RandomString(6))

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
		It("Should create a cluster with 3 control plane nodes", func() {
			By("Initializes with 3 control plane nodes")
			ApplyClusterTemplateAndWait(ctx, ApplyClusterTemplateAndWaitInput{
				ClusterProxy: bootstrapClusterProxy,
				ConfigCluster: clusterctl.ConfigClusterInput{
					LogFolder:                clusterctlLogFolder,
					ClusterctlConfigPath:     clusterctlConfigPath,
					KubeconfigPath:           bootstrapClusterProxy.GetKubeconfigPath(),
					InfrastructureProvider:   "docker",
					Flavor:                   "docker-move",
					Namespace:                namespace.Name,
					ClusterName:              clusterName,
					KubernetesVersion:        e2eConfig.MustGetVariable(KubernetesVersion),
					ControlPlaneMachineCount: ptr.To(int64(3)),
					WorkerMachineCount:       ptr.To(int64(1)),
					ClusterctlVariables:      map[string]string{"LOCAL_IMAGES": e2eConfig.MustGetVariable(LocalImages)},
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			WaitForClusterReady(ctx, WaitForClusterReadyInput{
				Getter:    bootstrapClusterProxy.GetClient(),
				Name:      result.Cluster.Name,
				Namespace: result.Cluster.Namespace,
			}, e2eConfig.GetIntervals(specName, "wait-cluster")...)

			WaitForAllMachinesRunningWithVersion(ctx, WaitForAllMachinesRunningWithVersionInput{
				Reader:      bootstrapClusterProxy.GetClient(),
				Version:     e2eConfig.MustGetVariable(KubernetesVersion),
				ClusterName: result.Cluster.Name,
				Namespace:   result.Cluster.Namespace,
			}, e2eConfig.GetIntervals(specName, "wait-cluster")...)

			preMoveMachineList := GetMachineNamesByCluster(ctx, GetMachinesByClusterInput{
				Lister:      bootstrapClusterProxy.GetClient(),
				ClusterName: result.Cluster.Name,
				Namespace:   result.Cluster.Namespace,
			})

			Byf("Using pivoted proxy with kubeconfig: %s", result.KubeconfigPath)
			pivotedProxy := framework.NewClusterProxy("pivoted", result.KubeconfigPath, initScheme())
			Expect(pivotedProxy).ToNot(BeNil(), "Failed to get a pivoted cluster proxy")

			By("Installing providers on workload cluster")
			InitManagementCluster(ctx, clusterctl.InitManagementClusterAndWatchControllerLogsInput{
				ClusterProxy:              pivotedProxy,
				ClusterctlConfigPath:      clusterctlConfigPath,
				InfrastructureProviders:   e2eConfig.InfrastructureProviders(),
				IPAMProviders:             e2eConfig.IPAMProviders(),
				RuntimeExtensionProviders: e2eConfig.RuntimeExtensionProviders(),
				BootstrapProviders:        []string{"rke2-bootstrap"},
				ControlPlaneProviders:     []string{"rke2-control-plane"},
				LogFolder:                 filepath.Join(artifactFolder, "clusters", pivotedProxy.GetName()),
				DisableMetricsCollection:  true,
			}, e2eConfig.GetIntervals(pivotedProxy.GetName(), "wait-controllers")...)

			Byf("Moving Cluster %s/%s from %s to %s", result.Cluster.Namespace, result.Cluster.Name, bootstrapClusterProxy.GetKubeconfigPath(), pivotedProxy.GetKubeconfigPath())
			clusterctl.Move(ctx, clusterctl.MoveInput{
				LogFolder:            clusterctlLogFolder,
				ClusterctlConfigPath: clusterctlConfigPath,
				Namespace:            namespace.Name,
				FromKubeconfigPath:   bootstrapClusterProxy.GetKubeconfigPath(),
				ToKubeconfigPath:     pivotedProxy.GetKubeconfigPath(),
			})

			WaitForControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       pivotedProxy.GetClient(),
				ControlPlane: client.ObjectKeyFromObject(result.ControlPlane),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			WaitForClusterReady(ctx, WaitForClusterReadyInput{
				Getter:    pivotedProxy.GetClient(),
				Name:      result.Cluster.Name,
				Namespace: result.Cluster.Namespace,
			}, e2eConfig.GetIntervals(specName, "wait-cluster")...)

			EnsureNoMachineRollout(ctx, GetMachinesByClusterInput{
				Lister:      pivotedProxy.GetClient(),
				ClusterName: result.Cluster.Name,
				Namespace:   result.Cluster.Namespace,
			}, preMoveMachineList)

			Byf("Scaling control planes to 1 and worker nodes to 3")
			ApplyClusterTemplateAndWait(ctx, ApplyClusterTemplateAndWaitInput{
				ClusterProxy: pivotedProxy,
				ConfigCluster: clusterctl.ConfigClusterInput{
					LogFolder:                clusterctlLogFolder,
					ClusterctlConfigPath:     clusterctlConfigPath,
					KubeconfigPath:           pivotedProxy.GetKubeconfigPath(),
					InfrastructureProvider:   "docker",
					Flavor:                   "docker-move",
					Namespace:                namespace.Name,
					ClusterName:              clusterName,
					KubernetesVersion:        e2eConfig.MustGetVariable(KubernetesVersion),
					ControlPlaneMachineCount: ptr.To(int64(1)),
					WorkerMachineCount:       ptr.To(int64(3)),
					ClusterctlVariables:      map[string]string{"LOCAL_IMAGES": e2eConfig.MustGetVariable(LocalImages)},
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			WaitForClusterReady(ctx, WaitForClusterReadyInput{
				Getter:    pivotedProxy.GetClient(),
				Name:      result.Cluster.Name,
				Namespace: result.Cluster.Namespace,
			}, e2eConfig.GetIntervals(specName, "wait-cluster")...)

			Byf("Scaling control planes to 3 and worker nodes to 1 with k8s version upgrade")
			ApplyClusterTemplateAndWait(ctx, ApplyClusterTemplateAndWaitInput{
				ClusterProxy: pivotedProxy,
				ConfigCluster: clusterctl.ConfigClusterInput{
					LogFolder:                clusterctlLogFolder,
					ClusterctlConfigPath:     clusterctlConfigPath,
					KubeconfigPath:           pivotedProxy.GetKubeconfigPath(),
					InfrastructureProvider:   "docker",
					Flavor:                   "docker-move",
					Namespace:                namespace.Name,
					ClusterName:              clusterName,
					KubernetesVersion:        e2eConfig.MustGetVariable(KubernetesVersionUpgradeTo),
					ControlPlaneMachineCount: ptr.To(int64(3)),
					WorkerMachineCount:       ptr.To(int64(1)),
					ClusterctlVariables:      map[string]string{"LOCAL_IMAGES": e2eConfig.MustGetVariable(LocalImages)},
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			WaitForClusterReady(ctx, WaitForClusterReadyInput{
				Getter:    pivotedProxy.GetClient(),
				Name:      result.Cluster.Name,
				Namespace: result.Cluster.Namespace,
			}, e2eConfig.GetIntervals(specName, "wait-cluster")...)

			WaitForAllMachinesRunningWithVersion(ctx, WaitForAllMachinesRunningWithVersionInput{
				Reader:      pivotedProxy.GetClient(),
				Version:     e2eConfig.MustGetVariable(KubernetesVersionUpgradeTo),
				ClusterName: result.Cluster.Name,
				Namespace:   result.Cluster.Namespace,
			}, e2eConfig.GetIntervals(specName, "wait-cluster")...)

			Byf("Moving back the Cluster %s/%s from %s to %s", result.Cluster.Namespace, result.Cluster.Name, pivotedProxy.GetKubeconfigPath(), bootstrapClusterProxy.GetKubeconfigPath())
			clusterctl.Move(ctx, clusterctl.MoveInput{
				LogFolder:            clusterctlLogFolder,
				ClusterctlConfigPath: clusterctlConfigPath,
				Namespace:            namespace.Name,
				FromKubeconfigPath:   pivotedProxy.GetKubeconfigPath(),
				ToKubeconfigPath:     bootstrapClusterProxy.GetKubeconfigPath(),
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
		})
	})
})
