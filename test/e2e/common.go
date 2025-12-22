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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

// Test suite constants for e2e config variables.
const (
	KubernetesVersionManagement     = "KUBERNETES_VERSION_MANAGEMENT"
	KubernetesVersion               = "KUBERNETES_VERSION"
	KubernetesVersionUpgradeTo      = "KUBERNETES_VERSION_UPGRADE_TO"
	CPMachineTemplateUpgradeTo      = "CONTROL_PLANE_MACHINE_TEMPLATE_UPGRADE_TO"
	WorkersMachineTemplateUpgradeTo = "WORKERS_MACHINE_TEMPLATE_UPGRADE_TO"
	ControlPlaneMachineCount        = "CONTROL_PLANE_MACHINE_COUNT"
	WorkerMachineCount              = "WORKER_MACHINE_COUNT"
	IPFamily                        = "IP_FAMILY"
	KindImageVersion                = "KIND_IMAGE_VERSION"
	LocalImages                     = "LOCAL_IMAGES"
)

func Byf(format string, a ...interface{}) {
	By(fmt.Sprintf(format, a...))
}

func setupSpecNamespace(ctx context.Context, specName string, clusterProxy framework.ClusterProxy, _ string) (*corev1.Namespace, context.CancelFunc) {
	Byf("Creating a namespace for hosting the %q test spec", specName)

	_, cancelWatches := context.WithCancel(ctx)
	return framework.CreateNamespace(ctx, framework.CreateNamespaceInput{Creator: clusterProxy.GetClient(), Name: fmt.Sprintf("%s-%s", specName, util.RandomString(6))}, "40s", "10s"), cancelWatches
}

func cleanupInstallation(ctx context.Context, clusterctlLogFolder, clusterctlConfigPath string, proxy framework.ClusterProxy) func() {
	return func() {
		By("Removing existing installations")
		clusterctl.Delete(ctx, clusterctl.DeleteInput{
			LogFolder:            clusterctlLogFolder,
			ClusterctlConfigPath: clusterctlConfigPath,
			KubeconfigPath:       proxy.GetKubeconfigPath(),
		})

		crd := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rke2controlplanes.controlplane.cluster.x-k8s.io",
			},
		}
		Expect(proxy.GetClient().Delete(ctx, crd)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(proxy.GetClient().Delete(ctx, crd)).ToNot(Succeed())
		}).Should(Succeed())

		crd = &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rke2configs.bootstrap.cluster.x-k8s.io",
			},
		}
		Expect(proxy.GetClient().Delete(ctx, crd)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(proxy.GetClient().Delete(ctx, crd)).ToNot(Succeed())
		}).Should(Succeed())

		crd = &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rke2configtemplates.bootstrap.cluster.x-k8s.io",
			},
		}
		Expect(proxy.GetClient().Delete(ctx, crd)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(proxy.GetClient().Delete(ctx, crd)).ToNot(Succeed())
		}).Should(Succeed())

		crd = &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rke2controlplanetemplates.controlplane.cluster.x-k8s.io",
			},
		}
		Expect(proxy.GetClient().Delete(ctx, crd)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(proxy.GetClient().Delete(ctx, crd)).ToNot(Succeed())
		}).Should(Succeed())
	}
}

type cleanupInput struct {
	SpecName             string
	ClusterProxy         framework.ClusterProxy
	ArtifactFolder       string
	Namespace            *corev1.Namespace
	CancelWatches        context.CancelFunc
	Cluster              *clusterv1.Cluster
	KubeconfigPath       string
	IntervalsGetter      func(spec, key string) []interface{}
	SkipCleanup          bool
	AdditionalCleanup    func()
	ClusterctlConfigPath string
}

func dumpSpecResourcesAndCleanup(ctx context.Context, input cleanupInput) {
	defer func() {
		input.CancelWatches()
	}()

	if input.Cluster == nil {
		By("Unable to dump workload cluster logs as the cluster is nil")
	} else {
		Byf("Dumping logs from the %q workload cluster", input.Cluster.Name)
		input.ClusterProxy.CollectWorkloadClusterLogs(ctx, input.Cluster.Namespace, input.Cluster.Name, filepath.Join(input.ArtifactFolder, "clusters", input.Cluster.Name))
	}

	Byf("Dumping all the Cluster API resources in the %q namespace", input.Namespace.Name)
	// Dump all Cluster API related resources to artifacts before deleting them.
	framework.DumpAllResources(ctx, framework.DumpAllResourcesInput{
		Lister:               input.ClusterProxy.GetClient(),
		KubeConfigPath:       input.ClusterProxy.GetKubeconfigPath(),
		ClusterctlConfigPath: input.ClusterctlConfigPath,
		Namespace:            input.Namespace.Name,
		LogPath:              filepath.Join(input.ArtifactFolder, "clusters", input.ClusterProxy.GetName(), "resources"),
	})

	if input.SkipCleanup {
		return
	}

	Byf("Deleting all clusters in the %s namespace", input.Namespace.Name)
	// While https://github.com/kubernetes-sigs/cluster-api/issues/2955 is addressed in future iterations, there is a chance
	// that cluster variable is not set even if the cluster exists, so we are calling DeleteAllClustersAndWait
	// instead of DeleteClusterAndWait
	framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
		ClusterProxy:         input.ClusterProxy,
		ClusterctlConfigPath: input.ClusterctlConfigPath,
		Namespace:            input.Namespace.Name,
	}, input.IntervalsGetter(input.SpecName, "wait-delete-cluster")...)

	Byf("Removing downstream Cluster kubeconfig: %s", input.KubeconfigPath)
	Expect(os.Remove(input.KubeconfigPath)).Should(Succeed())

	Byf("Deleting namespace used for hosting the %q test spec", input.SpecName)
	framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
		Deleter: input.ClusterProxy.GetClient(),
		Name:    input.Namespace.Name,
	})

	if input.AdditionalCleanup != nil {
		Byf("Running additional cleanup for the %q test spec", input.SpecName)
		input.AdditionalCleanup()
	}
}

func localLoadE2EConfig(configPath string) *clusterctl.E2EConfig {
	configData, err := os.ReadFile(configPath) //nolint:gosec
	Expect(err).ToNot(HaveOccurred(), "Failed to read the e2e test config file")
	Expect(configData).ToNot(BeEmpty(), "The e2e test config file should not be empty")

	config := &clusterctl.E2EConfig{}
	Expect(yaml.Unmarshal(configData, config)).To(Succeed(), "Failed to convert the e2e test config file to yaml")

	config.Defaults()
	config.AbsPaths(filepath.Dir(configPath))

	// TODO: this is the reason why we can't use this at present for the RKE2 tests
	// Expect(config.Validate()).To(Succeed(), "The e2e test config file is not valid")

	return config
}

// UpgradeManagementCluster upgrades provider a management cluster using clusterctl, and waits for the cluster to be ready.
func UpgradeManagementCluster(ctx context.Context, input clusterctl.UpgradeManagementClusterAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for UpgradeManagementCluster")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling UpgradeManagementCluster")
	Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling UpgradeManagementCluster")

	// Check if the user want a custom upgrade
	isCustomUpgrade := input.CoreProvider != "" ||
		len(input.BootstrapProviders) > 0 ||
		len(input.ControlPlaneProviders) > 0 ||
		len(input.InfrastructureProviders) > 0 ||
		len(input.IPAMProviders) > 0 ||
		len(input.RuntimeExtensionProviders) > 0 ||
		len(input.AddonProviders) > 0

	Expect((input.Contract != "" && !isCustomUpgrade) || (input.Contract == "" && isCustomUpgrade)).To(BeTrue(), `Invalid argument. Either the input.Contract parameter or at least one of the following providers has to be set:
		input.CoreProvider, input.BootstrapProviders, input.ControlPlaneProviders, input.InfrastructureProviders, input.IPAMProviders, input.RuntimeExtensionProviders, input.AddonProviders`)

	Expect(os.MkdirAll(input.LogFolder, 0o750)).To(Succeed(), "Invalid argument. input.LogFolder can't be created for UpgradeManagementClusterAndWait")

	upgradeInput := clusterctl.UpgradeInput{
		ClusterctlConfigPath:      input.ClusterctlConfigPath,
		ClusterctlVariables:       input.ClusterctlVariables,
		ClusterName:               input.ClusterProxy.GetName(),
		KubeconfigPath:            input.ClusterProxy.GetKubeconfigPath(),
		Contract:                  input.Contract,
		CoreProvider:              input.CoreProvider,
		BootstrapProviders:        input.BootstrapProviders,
		ControlPlaneProviders:     input.ControlPlaneProviders,
		InfrastructureProviders:   input.InfrastructureProviders,
		IPAMProviders:             input.IPAMProviders,
		RuntimeExtensionProviders: input.RuntimeExtensionProviders,
		AddonProviders:            input.AddonProviders,
		LogFolder:                 input.LogFolder,
	}

	clusterctl.Upgrade(ctx, upgradeInput)

	// We have to skip collecting metrics, as it causes failures in CI
}

// InitManagementCluster initializes a management using clusterctl.
func InitManagementCluster(ctx context.Context, input clusterctl.InitManagementClusterAndWatchControllerLogsInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for InitManagementCluster")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling InitManagementCluster")
	Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling InitManagementCluster")
	Expect(input.InfrastructureProviders).ToNot(BeEmpty(), "Invalid argument. input.InfrastructureProviders can't be empty when calling InitManagementCluster")
	Expect(os.MkdirAll(input.LogFolder, 0o750)).To(Succeed(), "Invalid argument. input.LogFolder can't be created for InitManagementCluster")

	logger := log.FromContext(ctx)

	if input.CoreProvider == "" {
		input.CoreProvider = config.ClusterAPIProviderName
	}
	if len(input.BootstrapProviders) == 0 {
		input.BootstrapProviders = []string{config.KubeadmBootstrapProviderName}
	}
	if len(input.ControlPlaneProviders) == 0 {
		input.ControlPlaneProviders = []string{config.KubeadmControlPlaneProviderName}
	}

	client := input.ClusterProxy.GetClient()
	controllersDeployments := framework.GetControllerDeployments(ctx, framework.GetControllerDeploymentsInput{
		Lister: client,
	})
	if len(controllersDeployments) == 0 {
		initInput := clusterctl.InitInput{
			// pass reference to the management cluster hosting this test
			KubeconfigPath: input.ClusterProxy.GetKubeconfigPath(),
			// pass the clusterctl config file that points to the local provider repository created for this test
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			// setup the desired list of providers for a single-tenant management cluster
			CoreProvider:              input.CoreProvider,
			BootstrapProviders:        input.BootstrapProviders,
			ControlPlaneProviders:     input.ControlPlaneProviders,
			InfrastructureProviders:   input.InfrastructureProviders,
			IPAMProviders:             input.IPAMProviders,
			RuntimeExtensionProviders: input.RuntimeExtensionProviders,
			AddonProviders:            input.AddonProviders,
			// setup clusterctl logs folder
			LogFolder: input.LogFolder,
		}

		clusterctl.Init(ctx, initInput)
	}

	logger.Info("Waiting for provider controllers to be running")

	controllersDeployments = framework.GetControllerDeployments(ctx, framework.GetControllerDeploymentsInput{
		Lister: client,
	})
	Expect(controllersDeployments).ToNot(BeEmpty(), "The list of controller deployments should not be empty")
	for _, deployment := range controllersDeployments {
		framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
			Getter:     client,
			Deployment: deployment,
		}, intervals...)
	}
}
