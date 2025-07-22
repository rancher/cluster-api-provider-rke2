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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/drone/envsubst/v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/cluster-api/util"
)

var _ = Describe("External Datastore", func() {
	var (
		specName            = "create-workload-cluster-with-external-datastore"
		namespace           *corev1.Namespace
		cancelWatches       context.CancelFunc
		result              *ApplyCustomClusterTemplateAndWaitResult
		clusterName         string
		clusterctlLogFolder string
	)

	BeforeEach(func() {
		Expect(e2eConfig).ToNot(BeNil(), "Invalid argument. e2eConfig can't be nil when calling %s spec", specName)
		Expect(clusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. clusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(bootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. bootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(artifactFolder, 0755)).To(Succeed(), "Invalid argument. artifactFolder can't be created for %s spec", specName)

		By("Initializing the bootstrap cluster")
		initBootstrapCluster(bootstrapClusterProxy, e2eConfig, clusterctlConfigPath, artifactFolder)

		clusterName = fmt.Sprintf("caprke2-e2e-%s", util.RandomString(6))

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
			SpecName:          specName,
			Cluster:           result.Cluster,
			KubeconfigPath:    result.KubeconfigPath,
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

	Context("Creating a cluster with an external datastore", func() {
		It("should create a workload cluster with 1 control-plane node and 1 worker node that can be scaled", func() {
			By("Creating a Postgres database in the management cluster")
			Apply(ctx, bootstrapClusterProxy, []byte(Postgres))
			KubectlWait(ctx, bootstrapClusterProxy.GetKubeconfigPath(), "--for=condition=Ready", "pod", "--selector=app=postgres", "--timeout=180s")

			By("Initializes with 1 CP and 1 worker node")
			resourceYaml, err := envsubst.Eval(string(ClusterTemplateDockerExternalDatastore), func(s string) string {
				switch s {
				case "CLUSTER_NAME":
					return clusterName
				case "NAMESPACE":
					return namespace.Name
				case "KUBERNETES_VERSION":
					return e2eConfig.GetVariable(KubernetesVersion)
				case "KIND_IMAGE_VERSION":
					return e2eConfig.GetVariable(KindImageVersion)
				case "CONTROL_PLANE_MACHINE_COUNT":
					return "1"
				case "WORKER_MACHINE_COUNT":
					return "1"
				default:
					return os.Getenv(s)
				}
			})
			Expect(err).ToNot(HaveOccurred())

			ApplyCustomClusterTemplateAndWait(ctx, ApplyCustomClusterTemplateAndWaitInput{
				ClusterProxy:                 bootstrapClusterProxy,
				CustomTemplateYAML:           []byte(resourceYaml),
				ClusterName:                  clusterName,
				Namespace:                    namespace.Name,
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			By("Scaling up control-plane nodes to 3")
			resourceYaml, err = envsubst.Eval(string(ClusterTemplateDockerExternalDatastore), func(s string) string {
				switch s {
				case "CLUSTER_NAME":
					return clusterName
				case "NAMESPACE":
					return namespace.Name
				case "KUBERNETES_VERSION":
					return e2eConfig.GetVariable(KubernetesVersion)
				case "KIND_IMAGE_VERSION":
					return e2eConfig.GetVariable(KindImageVersion)
				case "CONTROL_PLANE_MACHINE_COUNT":
					return "3"
				case "WORKER_MACHINE_COUNT":
					return "1"
				default:
					return os.Getenv(s)
				}
			})
			Expect(err).ToNot(HaveOccurred())

			ApplyCustomClusterTemplateAndWait(ctx, ApplyCustomClusterTemplateAndWaitInput{
				ClusterProxy:                 bootstrapClusterProxy,
				CustomTemplateYAML:           []byte(resourceYaml),
				ClusterName:                  clusterName,
				Namespace:                    namespace.Name,
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			By("Scaling down control-plane nodes to 1")
			resourceYaml, err = envsubst.Eval(string(ClusterTemplateDockerExternalDatastore), func(s string) string {
				switch s {
				case "CLUSTER_NAME":
					return clusterName
				case "NAMESPACE":
					return namespace.Name
				case "KUBERNETES_VERSION":
					return e2eConfig.GetVariable(KubernetesVersion)
				case "KIND_IMAGE_VERSION":
					return e2eConfig.GetVariable(KindImageVersion)
				case "CONTROL_PLANE_MACHINE_COUNT":
					return "1"
				case "WORKER_MACHINE_COUNT":
					return "1"
				default:
					return os.Getenv(s)
				}
			})
			Expect(err).ToNot(HaveOccurred())

			ApplyCustomClusterTemplateAndWait(ctx, ApplyCustomClusterTemplateAndWaitInput{
				ClusterProxy:                 bootstrapClusterProxy,
				CustomTemplateYAML:           []byte(resourceYaml),
				ClusterName:                  clusterName,
				Namespace:                    namespace.Name,
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)
		})
	})
})
