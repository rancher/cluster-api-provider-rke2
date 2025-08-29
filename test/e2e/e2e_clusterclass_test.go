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

	"github.com/drone/envsubst/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/cluster-api/util"
)

var _ = Describe("Cluster Class provisioning", func() {
	var (
		specName            = "cluster-class-provisioning"
		namespace           *corev1.Namespace
		cancelWatches       context.CancelFunc
		result              *ApplyCustomClusterTemplateAndWaitResult
		clusterName         string
		clusterClassName    string
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

		clusterName = fmt.Sprintf("caprke2-e2e-%s-clusterclass", util.RandomString(6))
		clusterClassName = "rke2-class"

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, bootstrapClusterProxy, artifactFolder)

		result = new(ApplyCustomClusterTemplateAndWaitResult)

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

	Context("Creating a Cluster using ClusterClass", func() {
		It("Should deploy a ClusterClass and create a Docker Cluster based on it", func() {
			By("Apply ClusterClass template")

			clusterClassConfig, err := envsubst.Eval(string(ClusterClassDocker), func(s string) string {
				switch s {
				case "CLASS_NAME":
					return clusterClassName
				case "NAMESPACE":
					return namespace.Name
				default:
					return os.Getenv(s)
				}
			})
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() error {
				return Apply(ctx, bootstrapClusterProxy, []byte(clusterClassConfig))
			}, e2eConfig.GetIntervals(specName, "wait-cluster")...).Should(Succeed(), "Failed to apply ClusterClass definition")

			By("Create a Docker Cluster from topology")

			clusterConfig, err := envsubst.Eval(string(ClusterFromClusterClassDocker), func(s string) string {
				switch s {
				case "CLUSTER_NAME":
					return clusterName
				case "CLASS_NAME":
					return clusterClassName
				case "NAMESPACE":
					return namespace.Name
				case "KUBERNETES_VERSION":
					return e2eConfig.MustGetVariable(KubernetesVersion)
				case "KIND_IMAGE_VERSION":
					return e2eConfig.MustGetVariable(KindImageVersion)
				case "CONTROL_PLANE_MACHINE_COUNT":
					return e2eConfig.MustGetVariable(ControlPlaneMachineCount)
				case "WORKER_MACHINE_COUNT":
					return e2eConfig.MustGetVariable(WorkerMachineCount)
				default:
					return os.Getenv(s)
				}
			})
			Expect(err).ToNot(HaveOccurred())

			ApplyCustomClusterTemplateAndWait(ctx, ApplyCustomClusterTemplateAndWaitInput{
				ClusterProxy:                 bootstrapClusterProxy,
				CustomTemplateYAML:           []byte(clusterConfig),
				ClusterName:                  clusterName,
				Namespace:                    namespace.Name,
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)
		})
	})
})
