//go:build e2e
// +build e2e

/*
Copyright 2023 SUSE.

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
	"k8s.io/utils/pointer"

	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
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

		clusterName = fmt.Sprintf("caprke2-e2e-%s-annotated", util.RandomString(6))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, bootstrapClusterProxy, artifactFolder)

		result = new(ApplyClusterTemplateAndWaitResult)

		// We need to override clusterctl apply log folder to avoid getting our credentials exposed.
		clusterctlLogFolder = filepath.Join(os.TempDir(), "clusters", bootstrapClusterProxy.GetName())
	})

	AfterEach(func() {
		cleanInput := cleanupInput{
			SpecName:        specName,
			Cluster:         result.Cluster,
			ClusterProxy:    bootstrapClusterProxy,
			Namespace:       namespace,
			CancelWatches:   cancelWatches,
			IntervalsGetter: e2eConfig.GetIntervals,
			SkipCleanup:     skipCleanup,
			ArtifactFolder:  artifactFolder,
		}

		dumpSpecResourcesAndCleanup(ctx, cleanInput)
	})

	Context("Creating a single control-plane cluster", func() {
		It("Should create a cluster with node annotations and set machine conditions", func() {
			By("Initializing with node annotations")
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
					KubernetesVersion:        e2eConfig.GetVariable(KubernetesVersion),
					ControlPlaneMachineCount: pointer.Int64Ptr(1),
					WorkerMachineCount:       pointer.Int64Ptr(1),
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			WaitForControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       bootstrapClusterProxy.GetClient(),
				ControlPlane: result.ControlPlane,
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			machineList := &clusterv1.MachineList{}
			Eventually(func() error {
				return bootstrapClusterProxy.GetClient().List(ctx, machineList, client.MatchingLabels(map[string]string{
					"node-role.kubernetes.io/master": "true",
				}))
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...).Should(Succeed())

			By("Waiting for machines to have propagated metadata")
			for _, machine := range machineList.Items {
				machine := machine

				WaitForMachineConditions(ctx, WaitForMachineConditionsInput{
					Getter:    bootstrapClusterProxy.GetClient(),
					Machine:   &machine,
					Checker:   conditions.IsTrue,
					Condition: controlplanev1.NodeMetadataUpToDate,
				}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)
			}
		})
	})
})
