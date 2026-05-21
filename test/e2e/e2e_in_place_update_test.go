//go:build e2e
// +build e2e

/*
Copyright 2026 SUSE.

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
	"slices"
	"time"

	"github.com/drone/envsubst/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
)

var _ = Describe("In-place update via Runtime Extension", Label(DefaultTestsLabel), func() {
	const (
		specName                  = "in-place-update"
		extensionConfigName       = "rke2-test-extension"
		extensionServiceNamespace = "rke2-test-extension-system"
		extensionServiceName      = "rke2-test-extension-webhook-service"
	)

	var (
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
		Expect(os.MkdirAll(artifactFolder, 0o755)).To(Succeed(), "Invalid argument. artifactFolder can't be created for %s spec", specName)
		Expect(e2eConfig.Variables).To(HaveKey(KubernetesVersion))

		By("Initializing the bootstrap cluster")
		initBootstrapCluster(bootstrapClusterProxy, e2eConfig, clusterctlConfigPath, artifactFolder)

		clusterName = fmt.Sprintf("caprke2-e2e-%s-%s", specName, util.RandomString(6))
		clusterClassName = fmt.Sprintf("rke2-in-place-%s", util.RandomString(6))

		namespace, cancelWatches = setupSpecNamespace(ctx, specName, bootstrapClusterProxy, artifactFolder)

		result = new(ApplyCustomClusterTemplateAndWaitResult)
		clusterctlLogFolder = filepath.Join(os.TempDir(), "clusters", bootstrapClusterProxy.GetName())
	})

	AfterEach(func() {
		err := CollectArtifacts(ctx, bootstrapClusterProxy.GetKubeconfigPath(), filepath.Join(artifactFolder, bootstrapClusterProxy.GetName(), clusterName+specName))
		Expect(err).ToNot(HaveOccurred())

		if !skipCleanup {
			Eventually(func() error {
				return bootstrapClusterProxy.GetClient().Delete(ctx, &runtimev1.ExtensionConfig{
					ObjectMeta: metav1.ObjectMeta{Name: extensionConfigName},
				})
			}, 10*time.Second, 1*time.Second).Should(Succeed(), "Failed to delete ExtensionConfig")
		}

		var clusterToCleanup *clusterv1.Cluster
		var kubeconfigPath string
		if result != nil {
			clusterToCleanup = result.Cluster
			kubeconfigPath = result.KubeconfigPath
		}
		dumpSpecResourcesAndCleanup(ctx, cleanupInput{
			SpecName:             specName,
			Cluster:              clusterToCleanup,
			KubeconfigPath:       kubeconfigPath,
			ClusterProxy:         bootstrapClusterProxy,
			Namespace:            namespace,
			CancelWatches:        cancelWatches,
			IntervalsGetter:      e2eConfig.GetIntervals,
			SkipCleanup:          skipCleanup,
			ArtifactFolder:       artifactFolder,
			AdditionalCleanup:    cleanupInstallation(ctx, clusterctlLogFolder, clusterctlConfigPath, bootstrapClusterProxy),
			ClusterctlConfigPath: clusterctlConfigPath,
		})
	})

	It("Should propagate topology `files` changes via in-place updates", func() {
		By("Creating an ExtensionConfig that targets the test-extension Service scoped to the test namespace")
		Expect(bootstrapClusterProxy.GetClient().Create(ctx, extensionConfig(
			extensionConfigName,
			extensionServiceNamespace,
			extensionServiceName,
			namespace.Name,
		))).To(Succeed())

		By("Applying the in-place ClusterClass")
		clusterClassConfig, err := envsubst.Eval(string(ClusterClassDockerInPlace), func(s string) string {
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
		}, e2eConfig.GetIntervals(specName, "wait-cluster")...).Should(Succeed(), "Failed to apply in-place ClusterClass")

		By("Creating a workload cluster from the in-place ClusterClass (3 CP machines)")
		clusterConfig, err := envsubst.Eval(string(ClusterFromClusterClassDockerInPlace), func(s string) string {
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
				return "3"
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

		cluster := result.Cluster
		mgmtClient := bootstrapClusterProxy.GetClient()

		Byf("Verifying Cluster %s is Available and Machines are Ready before starting in-place updates", cluster.Name)
		framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
			Getter:    mgmtClient,
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		})
		framework.VerifyMachinesReady(ctx, framework.VerifyMachinesReadyInput{
			Lister:    mgmtClient,
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		})

		machineListInput := GetMachinesByClusterInput{
			Lister:      mgmtClient,
			ClusterName: cluster.Name,
			Namespace:   cluster.Namespace,
		}

		verifyClusterConditionTrue := func(g Gomega, condType string) {
			latestCluster := &clusterv1.Cluster{}
			g.Expect(mgmtClient.Get(ctx, client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}, latestCluster)).To(Succeed())
			var status metav1.ConditionStatus
			var msg string
			for _, c := range latestCluster.Status.Conditions {
				if c.Type == condType {
					status = c.Status
					msg = c.Message
					break
				}
			}
			g.Expect(status).To(Equal(metav1.ConditionTrue), "Cluster condition %s should be True; message: %s", condType, msg)
		}

		By("Snapshotting Machine names before the first in-place update")
		beforeNames := GetMachineNamesByCluster(ctx, machineListInput)
		Expect(beforeNames).ToNot(BeEmpty(), "There must be at least one Machine")

		filePath := "/tmp/rke2-in-place-test"
		for i, fileContent := range []string{
			"first in-place update",
			"second in-place update",
			"third in-place update",
		} {
			Byf("[%d] Trigger in-place update by mutating the files topology variable", i+1)

			originalCluster := cluster.DeepCopy()

			cluster.Spec.Topology.Variables = slices.DeleteFunc(cluster.Spec.Topology.Variables, func(v clusterv1.ClusterVariable) bool {
				return v.Name == "files"
			})
			cluster.Spec.Topology.Variables = append(cluster.Spec.Topology.Variables, clusterv1.ClusterVariable{
				Name:  "files",
				Value: apiextensionsv1.JSON{Raw: []byte(fmt.Sprintf(`[{"path":%q,"content":%q}]`, filePath, fileContent))},
			})
			Expect(mgmtClient.Patch(ctx, cluster, client.MergeFrom(originalCluster))).To(Succeed())

			Eventually(func(g Gomega) {
				verifyClusterConditionTrue(g, string(clusterv1.ClusterControlPlaneMachinesUpToDateCondition))
				verifyClusterConditionTrue(g, string(clusterv1.ClusterWorkerMachinesUpToDateCondition))

				afterNames := GetMachineNamesByCluster(ctx, machineListInput)
				g.Expect(afterNames).To(ConsistOf(beforeNames))

				machineList := GetMachinesByCluster(ctx, machineListInput)
				for idx := range machineList.Items {
					m := &machineList.Items[idx]
					rke2Config := &bootstrapv1.RKE2Config{}
					g.Expect(mgmtClient.Get(ctx, client.ObjectKey{
						Namespace: m.Namespace,
						Name:      m.Spec.Bootstrap.ConfigRef.Name,
					}, rke2Config)).To(Succeed())
					g.Expect(rke2Config.Spec.Files).To(ContainElement(HaveField("Path", filePath)))
					g.Expect(rke2Config.Spec.Files).To(ContainElement(HaveField("Content", fileContent)))
				}
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...).Should(Succeed())
		}

		By("Triggering an in-place Kubernetes version upgrade")
		versionBefore := cluster.Spec.Topology.Version
		versionAfter := e2eConfig.MustGetVariable(KubernetesVersionUpgradeTo) + "+rke2r1"
		Expect(versionAfter).ToNot(Equal(versionBefore), "KUBERNETES_VERSION_UPGRADE_TO must differ from KUBERNETES_VERSION in the e2e config")

		originalCluster := cluster.DeepCopy()
		cluster.Spec.Topology.Version = versionAfter
		Expect(mgmtClient.Patch(ctx, cluster, client.MergeFrom(originalCluster))).To(Succeed())

		Eventually(func(g Gomega) {
			verifyClusterConditionTrue(g, string(clusterv1.ClusterControlPlaneMachinesUpToDateCondition))
			verifyClusterConditionTrue(g, string(clusterv1.ClusterWorkerMachinesUpToDateCondition))

			afterNames := GetMachineNamesByCluster(ctx, machineListInput)
			g.Expect(afterNames).To(ConsistOf(beforeNames), "version bump should be in-place — Machine names should not change")

			machineList := GetMachinesByCluster(ctx, machineListInput)
			for idx := range machineList.Items {
				g.Expect(machineList.Items[idx].Spec.Version).To(Equal(versionAfter), "Machine %s should have the new version", machineList.Items[idx].Name)
			}
		}, e2eConfig.GetIntervals(specName, "wait-control-plane")...).Should(Succeed())

		By("Triggering a non supported for in-place change and expecting rolling rollout")
		originalCluster = cluster.DeepCopy()
		cluster.Spec.Topology.Variables = slices.DeleteFunc(cluster.Spec.Topology.Variables, func(v clusterv1.ClusterVariable) bool {
			return v.Name == "preRKE2Commands"
		})
		cluster.Spec.Topology.Variables = append(cluster.Spec.Topology.Variables, clusterv1.ClusterVariable{
			Name:  "preRKE2Commands",
			Value: apiextensionsv1.JSON{Raw: []byte(`["echo in-place-test"]`)},
		})
		Expect(mgmtClient.Patch(ctx, cluster, client.MergeFrom(originalCluster))).To(Succeed())

		Eventually(func(g Gomega) {
			verifyClusterConditionTrue(g, string(clusterv1.ClusterControlPlaneMachinesUpToDateCondition))
			verifyClusterConditionTrue(g, string(clusterv1.ClusterWorkerMachinesUpToDateCondition))

			afterNames := GetMachineNamesByCluster(ctx, machineListInput)

			// Every original Machine must have been replaced — rolling rollout
			// completed (extension declined preRKE2Commands so in-place was not
			// possible).
			for _, before := range beforeNames {
				g.Expect(afterNames).ToNot(ContainElement(before),
					"expected original Machine %s to be replaced by rolling rollout", before)
			}
		}, e2eConfig.GetIntervals(specName, "wait-control-plane")...).Should(Succeed())
	})
})

func extensionConfig(name, serviceNamespace, serviceName string, namespaces ...string) *runtimev1.ExtensionConfig {
	cfg := &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				runtimev1.InjectCAFromSecretAnnotation: fmt.Sprintf("%s/%s-cert", serviceNamespace, serviceName),
			},
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				Service: runtimev1.ServiceReference{
					Namespace: serviceNamespace,
					Name:      serviceName,
				},
			},
		},
	}
	if len(namespaces) > 0 {
		cfg.Spec.NamespaceSelector = &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "kubernetes.io/metadata.name",
				Operator: metav1.LabelSelectorOpIn,
				Values:   namespaces,
			}},
		}
	}
	return cfg
}
