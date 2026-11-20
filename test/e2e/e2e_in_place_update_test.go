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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/drone/envsubst/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	planapi "github.com/rancher/rancher/pkg/plan"
	planv1alpha1 "github.com/rancher/rancher/pkg/plan/api/plan.cattle.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
)

var _ = Describe("In-place update via Runtime Extension", Label(InPlaceUpdatesTestsLabel), func() {
	const specName = "in-place-update"

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

	// The RKE2 extension relies on Rancher and system-agent, which only interact with it through a
	// machine-plan Secret per Machine. This behavior is mocked by the test: it creates the Secrets (like
	// Rancher) and reports the outcome of submitted plans (like system-agent).
	// The checks on Machines, RKE2Configs and conditions do not depend on system-agent, so they
	// validate the in-place updates support in CAPRKE2 for any extension.
	It("Should update Machines in-place, roll them out for unsupported changes, and stop when the update cannot be applied", func() {
		mgmtClient := bootstrapClusterProxy.GetClient()

		By("Enabling the RKE2 extension for the test namespace")
		originalNamespace := namespace.DeepCopy()
		metav1.SetMetaDataLabel(&namespace.ObjectMeta, "e2e.cluster.x-k8s.io/rke2-extension", "true")
		Expect(mgmtClient.Patch(ctx, namespace, client.MergeFrom(originalNamespace))).To(Succeed())

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

		verifyMachinesUpToDate := func(g Gomega) {
			latestCluster := &clusterv1.Cluster{}
			g.Expect(mgmtClient.Get(ctx, client.ObjectKeyFromObject(cluster), latestCluster)).To(Succeed())
			g.Expect(conditions.IsTrue(latestCluster, clusterv1.ClusterControlPlaneMachinesUpToDateCondition)).To(BeTrue(), "control plane Machines should be up-to-date")
			g.Expect(conditions.IsTrue(latestCluster, clusterv1.ClusterWorkerMachinesUpToDateCondition)).To(BeTrue(), "worker Machines should be up-to-date")
		}

		setVariable := func(name, value string) {
			originalCluster := cluster.DeepCopy()
			cluster.Spec.Topology.Variables = slices.DeleteFunc(cluster.Spec.Topology.Variables, func(v clusterv1.ClusterVariable) bool {
				return v.Name == name
			})
			cluster.Spec.Topology.Variables = append(cluster.Spec.Topology.Variables, clusterv1.ClusterVariable{
				Name:  name,
				Value: apiextensionsv1.JSON{Raw: []byte(value)},
			})
			Expect(mgmtClient.Patch(ctx, cluster, client.MergeFrom(originalCluster))).To(Succeed())
		}

		// waitForInPlaceUpdateToStop acts as system-agent failing to perform the update until
		// the extension reports the failure, then checks for a while that no Machine is replaced.
		waitForInPlaceUpdateToStop := func(machineNames []string, failure string) {
			Eventually(func(g Gomega) {
				actAsSystemAgent(g, mgmtClient, namespace.Name, planapi.PlanStateFailed)

				events := &corev1.EventList{}
				g.Expect(mgmtClient.List(ctx, events, client.InNamespace(namespace.Name))).To(Succeed())
				g.Expect(events.Items).To(ContainElement(And(
					HaveField("Reason", "InPlaceUpdateFailed"),
					HaveField("Message", ContainSubstring(failure)),
				)), "the extension should report %q with an event", failure)
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...).Should(Succeed())

			Consistently(func(g Gomega) {
				g.Expect(GetMachineNamesByCluster(ctx, machineListInput)).To(ConsistOf(machineNames), "Machines should not be replaced when the in-place update stops")
			}, 2*time.Minute, 10*time.Second).Should(Succeed())
		}

		By("Snapshotting Machine names before the rollout")
		beforeNames := GetMachineNamesByCluster(ctx, machineListInput)
		Expect(beforeNames).ToNot(BeEmpty(), "There must be at least one Machine")

		By("Triggering a non supported in-place edit and expecting a rollout")
		setVariable("preRKE2Commands", `["echo in-place-test"]`)

		Eventually(func(g Gomega) {
			verifyMachinesUpToDate(g)

			// With maxSurge 0 a Machine is removed before its replacement exists, so also wait for
			// all replacements to be created and Ready.
			machines := GetMachinesByCluster(ctx, machineListInput).Items
			g.Expect(machines).To(HaveLen(len(beforeNames)), "all replacement Machines should exist")
			for i := range machines {
				g.Expect(beforeNames).ToNot(ContainElement(machines[i].Name), "Machine %s should have been replaced by a rollout", machines[i].Name)
				g.Expect(conditions.IsTrue(&machines[i], clusterv1.MachineReadyCondition)).To(BeTrue(), "Machine %s should be Ready", machines[i].Name)
			}
		}, e2eConfig.GetIntervals(specName, "wait-control-plane")...).Should(Succeed())

		// The new Machines have no machine-plan Secret yet (system-agent did not register).
		machineNames := GetMachineNamesByCluster(ctx, machineListInput)

		By("Changing a file while the Machines have no machine-plan Secret, and expecting the update to stop")
		filePath := "/tmp/rke2-in-place-test"
		fileContent := "in-place update"
		setVariable("files", fmt.Sprintf(`[{"path":%q,"content":%q}]`, filePath, fileContent))
		waitForInPlaceUpdateToStop(machineNames, "machine-plan secret not found")

		By("Acting as Rancher: creating a machine-plan Secret for every Machine, and expecting the update to complete in-place")
		createMachinePlanSecrets(mgmtClient, namespace.Name, machineNames)

		Eventually(func(g Gomega) {
			actAsSystemAgent(g, mgmtClient, namespace.Name, planapi.PlanStateSucceeded)
			verifyMachinesUpToDate(g)
			g.Expect(GetMachineNamesByCluster(ctx, machineListInput)).To(ConsistOf(machineNames), "Machines should be updated in-place, not replaced")

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

				// The plan must install the Machine's version and write the file.
				secret := &corev1.Secret{}
				g.Expect(mgmtClient.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: m.Name + "-machine-plan"}, secret)).To(Succeed())
				plan := planapi.Plan{}
				g.Expect(json.Unmarshal(secret.Data["plan"], &plan)).To(Succeed())
				g.Expect(plan.OneTimeInstructions).To(ContainElement(HaveField("Image",
					"rancher/system-agent-installer-rke2:"+strings.ReplaceAll(m.Spec.Version, "+", "-"))))
				g.Expect(plan.Files).To(ContainElement(HaveField("Path", filePath)))
			}
		}, e2eConfig.GetIntervals(specName, "wait-control-plane")...).Should(Succeed())

		By("Changing a file again, letting system-agent fail the plans, and expecting the update to stop")
		setVariable("files", fmt.Sprintf(`[{"path":%q,"content":"failed in-place update"}]`, filePath))
		waitForInPlaceUpdateToStop(machineNames, "system-agent failed to apply the update plan")
	})
})

// createMachinePlanSecrets creates a machine-plan Secret for every given Machine, the same way
// Rancher does once system-agent registers.
func createMachinePlanSecrets(c client.Client, namespace string, machineNames []string) {
	for _, name := range machineNames {
		Expect(c.Create(ctx, &corev1.Secret{
			Name:      name + "-machine-plan",
			Namespace: namespace,
			Labels: map[string]string{
				planv1alpha1.MachineLifecycleGroupLabel: clusterv1.GroupVersion.Group,
				planv1alpha1.MachineLifecycleKindLabel:  "Machine",
				planv1alpha1.MachineLifecycleNameLabel:  name,
			},
			Type: planapi.SecretTypeMachinePlan,
		})).To(Succeed())
	}
}

// actAsSystemAgent reports the result for every plan submitted to the machine-plan Secrets
// in the namespace, the same way system-agent does after running a plan.
func actAsSystemAgent(g Gomega, c client.Client, namespace string, outcome planapi.PlanState) {
	checksumKey := "applied-checksum"
	if outcome == planapi.PlanStateFailed {
		checksumKey = "failed-checksum"
	}

	secrets := &corev1.SecretList{}
	g.Expect(c.List(ctx, secrets, client.InNamespace(namespace), client.HasLabels{planv1alpha1.MachineLifecycleNameLabel})).To(Succeed())

	for i := range secrets.Items {
		secret := &secrets.Items[i]
		plan := secret.Data["plan"]
		if len(plan) == 0 || string(secret.Data[checksumKey]) == planapi.Checksum(plan) {
			continue
		}

		original := secret.DeepCopy()
		secret.Data[checksumKey] = []byte(planapi.Checksum(plan))
		secret.Data[planapi.PlanStateKey] = []byte(outcome)
		g.Expect(c.Patch(ctx, secret, client.MergeFrom(original))).To(Succeed())
	}
}
