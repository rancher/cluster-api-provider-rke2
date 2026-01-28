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
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	dockerv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NOTE: the code in this file is largely copied from the cluster-api test framework with
// modifications so that Kubeadm Control Plane isn't used.
// Source: sigs.k8s.io/cluster-api/test/framework/*

const (
	retryableOperationInterval = 3 * time.Second
	retryableOperationTimeout  = 3 * time.Minute
)

// ApplyClusterTemplateAndWaitInput is the input type for ApplyClusterTemplateAndWait.
type ApplyClusterTemplateAndWaitInput struct {
	ClusterProxy                 framework.ClusterProxy
	ConfigCluster                clusterctl.ConfigClusterInput
	WaitForClusterIntervals      []interface{}
	WaitForControlPlaneIntervals []interface{}
	WaitForMachineDeployments    []interface{}
	Args                         []string
	PreWaitForCluster            func()
	PostMachinesProvisioned      func()
	// SkipPreviousVersionChecks can be used when introducing a new feature that does not exist in the previous version.
	SkipPreviousVersionChecks bool
	ControlPlaneWaiters
}

// ApplyCustomClusterTemplateAndWaitInput is the input type for ApplyCustomClusterTemplateAndWait.
type ApplyCustomClusterTemplateAndWaitInput struct {
	ClusterProxy                 framework.ClusterProxy
	CustomTemplateYAML           []byte
	ClusterName                  string
	Namespace                    string
	WaitForClusterIntervals      []interface{}
	WaitForControlPlaneIntervals []interface{}
	WaitForMachineDeployments    []interface{}
	Args                         []string
	PreWaitForCluster            func()
	PostMachinesProvisioned      func()
	SkipPreviousVersionChecks    bool
	ControlPlaneWaiters
}

// Waiter is a function that runs and waits for a long-running operation to finish and updates the result.
type Waiter func(ctx context.Context, input ApplyCustomClusterTemplateAndWaitInput, result *ApplyCustomClusterTemplateAndWaitResult)

// ControlPlaneWaiters are Waiter functions for the control plane.
type ControlPlaneWaiters struct {
	WaitForControlPlaneInitialized   Waiter
	WaitForControlPlaneMachinesReady Waiter
}

// ApplyClusterTemplateAndWaitResult is the output type for ApplyClusterTemplateAndWait.
type ApplyClusterTemplateAndWaitResult struct {
	ClusterClass       *clusterv1.ClusterClass
	Cluster            *clusterv1.Cluster
	ControlPlane       *controlplanev1.RKE2ControlPlane
	MachineDeployments []*clusterv1.MachineDeployment
	KubeconfigPath     string
}

// ApplyCustomClusterTemplateAndWaitResult is the output type for ApplyCustomClusterTemplateAndWait.
type ApplyCustomClusterTemplateAndWaitResult struct {
	ClusterClass       *clusterv1.ClusterClass
	Cluster            *clusterv1.Cluster
	ControlPlane       *controlplanev1.RKE2ControlPlane
	MachineDeployments []*clusterv1.MachineDeployment
	KubeconfigPath     string
}

// ApplyClusterTemplateAndWait gets a managed cluster template using clusterctl, and waits for the cluster to be ready.
// Important! this method assumes the cluster uses a RKE2ControlPlane and MachineDeployment.
func ApplyClusterTemplateAndWait(ctx context.Context, input ApplyClusterTemplateAndWaitInput, result *ApplyClusterTemplateAndWaitResult) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for ApplyClusterTemplateAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling ApplyManagedClusterTemplateAndWait")
	Expect(result).ToNot(BeNil(), "Invalid argument. result can't be nil when calling ApplyClusterTemplateAndWait")
	Expect(input.ConfigCluster.Flavor).ToNot(BeEmpty(), "Invalid argument. input.ConfigCluster.Flavor can't be empty")
	Expect(input.ConfigCluster.ControlPlaneMachineCount).ToNot(BeNil())
	Expect(input.ConfigCluster.WorkerMachineCount).ToNot(BeNil())

	By(fmt.Sprintf("Creating the RKE2 based workload cluster with name %q using the %q template (Kubernetes %s)",
		input.ConfigCluster.ClusterName, input.ConfigCluster.Flavor, input.ConfigCluster.KubernetesVersion))

	By("Getting the cluster template yaml")
	workloadClusterTemplate := clusterctl.ConfigCluster(ctx, clusterctl.ConfigClusterInput{
		// pass reference to the management cluster hosting this test
		KubeconfigPath: input.ConfigCluster.KubeconfigPath,
		// pass the clusterctl config file that points to the local provider repository created for this test,
		ClusterctlConfigPath: input.ConfigCluster.ClusterctlConfigPath,
		// select template
		Flavor: input.ConfigCluster.Flavor,
		// define template variables
		Namespace:                input.ConfigCluster.Namespace,
		ClusterName:              input.ConfigCluster.ClusterName,
		KubernetesVersion:        input.ConfigCluster.KubernetesVersion,
		ControlPlaneMachineCount: input.ConfigCluster.ControlPlaneMachineCount,
		WorkerMachineCount:       input.ConfigCluster.WorkerMachineCount,
		InfrastructureProvider:   input.ConfigCluster.InfrastructureProvider,
		// setup clusterctl logs folder
		LogFolder:           input.ConfigCluster.LogFolder,
		ClusterctlVariables: input.ConfigCluster.ClusterctlVariables,
	})
	Expect(workloadClusterTemplate).ToNot(BeNil(), "Failed to get the cluster template")

	ApplyCustomClusterTemplateAndWait(ctx, ApplyCustomClusterTemplateAndWaitInput{
		ClusterProxy:                 input.ClusterProxy,
		CustomTemplateYAML:           workloadClusterTemplate,
		ClusterName:                  input.ConfigCluster.ClusterName,
		Namespace:                    input.ConfigCluster.Namespace,
		WaitForClusterIntervals:      input.WaitForClusterIntervals,
		WaitForControlPlaneIntervals: input.WaitForControlPlaneIntervals,
		WaitForMachineDeployments:    input.WaitForMachineDeployments,
		PreWaitForCluster:            input.PreWaitForCluster,
		PostMachinesProvisioned:      input.PostMachinesProvisioned,
		ControlPlaneWaiters:          input.ControlPlaneWaiters,
		SkipPreviousVersionChecks:    input.SkipPreviousVersionChecks,
	}, (*ApplyCustomClusterTemplateAndWaitResult)(result))
}

// ApplyCustomClusterTemplateAndWait deploys a cluster from a custom yaml file, and waits for the cluster to be ready.
// Important! this method assumes the cluster uses a RKE2ControlPlane and MachineDeployment.
func ApplyCustomClusterTemplateAndWait(ctx context.Context, input ApplyCustomClusterTemplateAndWaitInput, result *ApplyCustomClusterTemplateAndWaitResult) {
	setDefaults(&input)
	Expect(ctx).NotTo(BeNil(), "ctx is required for ApplyCustomClusterTemplateAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling ApplyCustomClusterTemplateAndWait")
	Expect(input.CustomTemplateYAML).NotTo(BeEmpty(), "Invalid argument. input.CustomTemplateYAML can't be empty when calling ApplyCustomClusterTemplateAndWait")
	Expect(input.ClusterName).NotTo(BeEmpty(), "Invalid argument. input.ClusterName can't be empty when calling ApplyCustomClusterTemplateAndWait")
	Expect(input.Namespace).NotTo(BeEmpty(), "Invalid argument. input.Namespace can't be empty when calling ApplyCustomClusterTemplateAndWait")
	Expect(result).ToNot(BeNil(), "Invalid argument. result can't be nil when calling ApplyCustomClusterTemplateAndWait")

	By(fmt.Sprintf("Creating the workload cluster with name %q from the provided yaml", input.ClusterName))

	By(fmt.Sprintf("Applying the cluster template yaml of cluster %s", klog.KRef(input.Namespace, input.ClusterName)))
	Eventually(func() error {
		return Apply(ctx, input.ClusterProxy, input.CustomTemplateYAML, input.Args...)
	}, input.WaitForClusterIntervals...).Should(Succeed(), "Failed to apply the cluster template")

	// Once we applied the cluster template we can run PreWaitForCluster.
	// Note: This can e.g. be used to verify the BeforeClusterCreate lifecycle hook is executed
	// and blocking correctly.
	if input.PreWaitForCluster != nil {
		By(fmt.Sprintf("Calling PreWaitForCluster for cluster %s", klog.KRef(input.Namespace, input.ClusterName)))
		input.PreWaitForCluster()
	}

	By(fmt.Sprintf("Waiting for the cluster infrastructure of cluster %s to be provisioned", klog.KRef(input.Namespace, input.ClusterName)))
	result.Cluster = framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
		Getter:    input.ClusterProxy.GetClient(),
		Namespace: input.Namespace,
		Name:      input.ClusterName,
	}, input.WaitForClusterIntervals...)

	if result.Cluster.Spec.Topology.IsDefined() {
		result.ClusterClass = framework.GetClusterClassByName(ctx, framework.GetClusterClassByNameInput{
			Getter:    input.ClusterProxy.GetClient(),
			Namespace: input.Namespace,
			Name:      result.Cluster.Spec.Topology.ClassRef.Name,
		})
	}

	By(fmt.Sprintf("Waiting for control plane of cluster %s to be initialized", klog.KRef(input.Namespace, input.ClusterName)))
	input.WaitForControlPlaneInitialized(ctx, input, result)

	By(fmt.Sprintf("Waiting for control plane of cluster %s to be ready", klog.KRef(input.Namespace, input.ClusterName)))
	input.WaitForControlPlaneMachinesReady(ctx, input, result)

	By(fmt.Sprintf("Waiting for the machine deployments of cluster %s to be provisioned", klog.KRef(input.Namespace, input.ClusterName)))
	result.MachineDeployments = framework.DiscoveryAndWaitForMachineDeployments(ctx, framework.DiscoveryAndWaitForMachineDeploymentsInput{
		Lister:  input.ClusterProxy.GetClient(),
		Cluster: result.Cluster,
	}, input.WaitForMachineDeployments...)

	if input.PostMachinesProvisioned != nil {
		By(fmt.Sprintf("Calling PostMachinesProvisioned for cluster %s", klog.KRef(input.Namespace, input.ClusterName)))
		input.PostMachinesProvisioned()
	}

	VerifyDockerMachineTemplateOwner(ctx, VerifyDockerMachineTemplateOwnerInput{
		Lister:      input.ClusterProxy.GetClient(),
		ClusterName: result.Cluster.Name,
		Namespace:   result.Cluster.Namespace,
	})

	// Dump the kubeconfig path if not done previously.
	if result.KubeconfigPath == "" {
		result.KubeconfigPath = WriteKubeconfigSecretToDisk(ctx, input.ClusterProxy.GetClient(), result.Cluster.Namespace, result.Cluster.Name+"-kubeconfig")
	}
}

func WriteKubeconfigSecretToDisk(ctx context.Context, client framework.Getter, namespace, name string) string {
	Byf("Dumping Cluster kubeconfig secret %s/%s", namespace, name)
	kubeconfigSecret := &corev1.Secret{}
	Expect(client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, kubeconfigSecret)).Should(Succeed())
	content, ok := kubeconfigSecret.Data["value"]
	Expect(ok).To(BeTrue(), "Failed to find expected key in kubeconfig secret")
	cfg, err := clientcmd.Load(content)
	Expect(err).ShouldNot(HaveOccurred(), "Failed to unmarshall data into kubeconfig")
	tempFile, err := os.CreateTemp("", "e2e-kubeconfig")
	Expect(err).NotTo(HaveOccurred(), "Failed to create temp file for kubeconfig")

	Byf("Writing kubeconfig to temp file %s", tempFile.Name())
	err = clientcmd.WriteToFile(*cfg, tempFile.Name())
	Expect(err).ShouldNot(HaveOccurred(), "Failed to write kubeconfig to file %s", tempFile.Name())
	return tempFile.Name()
}

// DiscoveryAndWaitForRKE2ControlPlaneInitializedInput is the input type for DiscoveryAndWaitForRKE2ControlPlaneInitialized.
type DiscoveryAndWaitForRKE2ControlPlaneInitializedInput struct {
	Lister  framework.Lister
	Cluster *clusterv1.Cluster
}

// DiscoveryAndWaitForRKE2ControlPlaneInitialized discovers the RKE2 object attached to a cluster and waits for it to be initialized.
func DiscoveryAndWaitForRKE2ControlPlaneInitialized(ctx context.Context, input DiscoveryAndWaitForRKE2ControlPlaneInitializedInput, intervals ...interface{}) *controlplanev1.RKE2ControlPlane {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DiscoveryAndWaitForRKE2ControlPlaneInitialized")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling DiscoveryAndWaitForRKE2ControlPlaneInitialized")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling DiscoveryAndWaitForRKE2ControlPlaneInitialized")

	By("Getting RKE2ControlPlane control plane")

	var controlPlane *controlplanev1.RKE2ControlPlane
	Eventually(func(g Gomega) {
		controlPlane = GetRKE2ControlPlaneByCluster(ctx, GetRKE2ControlPlaneByClusterInput{
			Lister:      input.Lister,
			ClusterName: input.Cluster.Name,
			Namespace:   input.Cluster.Namespace,
		})
		g.Expect(controlPlane).ToNot(BeNil())
	}, "2m", "1s").Should(Succeed(), "Couldn't get the control plane for the cluster %s", klog.KObj(input.Cluster))

	return controlPlane
}

// GetRKE2ControlPlaneByClusterInput is the input for GetRKE2ControlPlaneByCluster.
type GetRKE2ControlPlaneByClusterInput struct {
	Lister      framework.Lister
	ClusterName string
	Namespace   string
}

// GetRKE2ControlPlaneByCluster returns the RKE2ControlPlane objects for a cluster.
func GetRKE2ControlPlaneByCluster(ctx context.Context, input GetRKE2ControlPlaneByClusterInput) *controlplanev1.RKE2ControlPlane {
	opts := []client.ListOption{
		client.InNamespace(input.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel: input.ClusterName,
		},
	}

	controlPlaneList := &controlplanev1.RKE2ControlPlaneList{}
	Eventually(func() error {
		return input.Lister.List(ctx, controlPlaneList, opts...)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list RKE2ControlPlane object for Cluster %s", klog.KRef(input.Namespace, input.ClusterName))
	Expect(len(controlPlaneList.Items)).ToNot(BeNumerically(">", 1), "Cluster %s should not have more than 1 RKE2ControlPlane object", klog.KRef(input.Namespace, input.ClusterName))
	if len(controlPlaneList.Items) == 1 {
		return &controlPlaneList.Items[0]
	}
	return nil
}

// GetMachinesByClusterInput is the input for GetRKE2ControlPlaneByCluster.
type GetMachinesByClusterInput struct {
	Lister      framework.Lister
	ClusterName string
	Namespace   string
}

// GetMachinesByCluster returns the Machine objects for a cluster.
func GetMachinesByCluster(ctx context.Context, input GetMachinesByClusterInput) *clusterv1.MachineList {
	opts := []client.ListOption{
		client.InNamespace(input.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel: input.ClusterName,
		},
	}

	machineList := &clusterv1.MachineList{}
	Eventually(func() error {
		return input.Lister.List(ctx, machineList, opts...)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list Machine objects for Cluster %s", klog.KRef(input.Namespace, input.ClusterName))

	return machineList
}

// GetMachinesByCluster returns the Machine objects for a cluster.
func GetMachineNamesByCluster(ctx context.Context, input GetMachinesByClusterInput) []string {
	machineList := GetMachinesByCluster(ctx, input)

	machineNames := []string{}
	for _, machine := range machineList.Items {
		machineNames = append(machineNames, machine.Name)
	}
	return machineNames
}

// GetMachinesByCluster returns the Machine objects for a cluster.
func GetMachineNamesByClusterv1Beta1(ctx context.Context, input GetMachinesByClusterInput) []string {
	opts := []client.ListOption{
		client.InNamespace(input.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel: input.ClusterName,
		},
	}

	machineList := &clusterv1beta1.MachineList{}
	Eventually(func() error {
		return input.Lister.List(ctx, machineList, opts...)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list Machine objects for Cluster %s", klog.KRef(input.Namespace, input.ClusterName))

	machineNames := []string{}
	for _, machine := range machineList.Items {
		machineNames = append(machineNames, machine.Name)
	}
	return machineNames
}

// WaitForControlPlaneAndMachinesReadyInput is the input type for WaitForControlPlaneAndMachinesReady.
type WaitForControlPlaneAndMachinesReadyInput struct {
	GetLister    framework.GetLister
	Cluster      *clusterv1.Cluster
	ControlPlane *controlplanev1.RKE2ControlPlane
}

// WaitForControlPlaneAndMachinesReady waits for a RKE2ControlPlane object to be ready (all the machine provisioned and one node ready).
func WaitForControlPlaneAndMachinesReady(ctx context.Context, input WaitForControlPlaneAndMachinesReadyInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForControlPlaneReady")
	Expect(input.GetLister).ToNot(BeNil(), "Invalid argument. input.GetLister can't be nil when calling WaitForControlPlaneReady")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling WaitForControlPlaneReady")
	Expect(input.ControlPlane).ToNot(BeNil(), "Invalid argument. input.ControlPlane can't be nil when calling WaitForControlPlaneReady")

	if input.ControlPlane.Spec.Replicas != nil && int(*input.ControlPlane.Spec.Replicas) > 1 {
		By(fmt.Sprintf("Waiting for the remaining control plane machines managed by %s to be provisioned", klog.KObj(input.ControlPlane)))
		WaitForRKE2ControlPlaneMachinesToExist(ctx, WaitForRKE2ControlPlaneMachinesToExistInput{
			Lister:       input.GetLister,
			Cluster:      input.Cluster,
			ControlPlane: input.ControlPlane,
		}, intervals...)
	}

	By(fmt.Sprintf("Waiting for control plane %s to be ready (implies underlying nodes to be ready as well)", klog.KObj(input.ControlPlane)))
	waitForControlPlaneToBeReadyInput := WaitForControlPlaneToBeReadyInput{
		Getter:       input.GetLister,
		ControlPlane: client.ObjectKeyFromObject(input.ControlPlane),
	}
	WaitForControlPlaneToBeReady(ctx, waitForControlPlaneToBeReadyInput, intervals...)

	framework.AssertControlPlaneFailureDomains(ctx, framework.AssertControlPlaneFailureDomainsInput{
		Lister:  input.GetLister,
		Cluster: input.Cluster,
	})
}

// WaitForRKE2ControlPlaneMachinesToExistInput is the input for WaitForRKE2ControlPlaneMachinesToExist.
type WaitForRKE2ControlPlaneMachinesToExistInput struct {
	Lister       framework.Lister
	Cluster      *clusterv1.Cluster
	ControlPlane *controlplanev1.RKE2ControlPlane
}

// WaitForRKE2ControlPlaneMachinesToExist will wait until all control plane machines have node refs.
func WaitForRKE2ControlPlaneMachinesToExist(ctx context.Context, input WaitForRKE2ControlPlaneMachinesToExistInput, intervals ...interface{}) {
	By("Waiting for all control plane nodes to exist")
	inClustersNamespaceListOption := client.InNamespace(input.Cluster.Namespace)
	// ControlPlane labels
	matchClusterListOption := client.MatchingLabels{
		clusterv1.MachineControlPlaneLabel: "",
		clusterv1.ClusterNameLabel:         input.Cluster.Name,
	}

	Eventually(func() (int, error) {
		machineList := &clusterv1.MachineList{}
		if err := input.Lister.List(ctx, machineList, inClustersNamespaceListOption, matchClusterListOption); err != nil {
			By(fmt.Sprintf("Failed to list the machines: %+v", err))
			return 0, err
		}
		count := 0
		for _, machine := range machineList.Items {
			if machine.Status.NodeRef.IsDefined() {
				count++
			}
		}
		return count, nil
	}, intervals...).Should(Equal(int(*input.ControlPlane.Spec.Replicas)), "Timed out waiting for %d control plane machines to exist", int(*input.ControlPlane.Spec.Replicas))

	// check if machines owned by RKE2ControlPlane have the labels provided in .spec.machineTemplate.metadata.labels
	controlPlaneMachineTemplateLabels := input.ControlPlane.Spec.MachineTemplate.ObjectMeta.Labels
	machineList := &clusterv1.MachineList{}
	err := input.Lister.List(ctx, machineList, inClustersNamespaceListOption, matchClusterListOption)
	Expect(err).To(BeNil(), "failed to list the machines")

	for _, machine := range machineList.Items {
		machineLabels := machine.ObjectMeta.Labels
		for k := range controlPlaneMachineTemplateLabels {
			Expect(machineLabels[k]).To(Equal(controlPlaneMachineTemplateLabels[k]))
		}
	}
}

// WaitForControlPlaneToBeReadyInput is the input for WaitForControlPlaneToBeReady.
type WaitForControlPlaneToBeReadyInput struct {
	Getter       framework.Getter
	ControlPlane types.NamespacedName
}

// WaitForControlPlaneToBeReady will wait for a control plane to be ready.
func WaitForControlPlaneToBeReady(ctx context.Context, input WaitForControlPlaneToBeReadyInput, intervals ...interface{}) {
	By("Waiting for the control plane to be ready")
	controlplane := &controlplanev1.RKE2ControlPlane{}
	Eventually(func() (bool, error) {
		key := client.ObjectKey{
			Namespace: input.ControlPlane.Namespace,
			Name:      input.ControlPlane.Name,
		}
		if err := input.Getter.Get(ctx, key, controlplane); err != nil {
			return false, errors.Wrapf(err, "failed to get RKE2 control plane")
		}

		desiredReplicas := controlplane.Spec.Replicas
		statusReplicas := controlplane.Status.Replicas
		upToDateReplicas := controlplane.Status.UpToDateReplicas
		readyReplicas := controlplane.Status.ReadyReplicas

		// Control plane is still rolling out (and thus not ready) if:
		// * .spec.replicas, .status.replicas, .status.upToDatedReplicas,
		//   .status.readyReplicas are not equal
		if !ptr.Equal(statusReplicas, desiredReplicas) ||
			!ptr.Equal(upToDateReplicas, desiredReplicas) ||
			!ptr.Equal(readyReplicas, desiredReplicas) {
			return false, nil
		}

		return true, nil
	}, intervals...).Should(BeTrue(), framework.PrettyPrint(controlplane)+"\n")
}

type WaitForMachineConditionsInput struct {
	Getter    framework.Getter
	Machine   *clusterv1.Machine
	Checker   func(_ conditions.Getter, _ string) bool
	Condition string
}

func WaitForMachineConditions(ctx context.Context, input WaitForMachineConditionsInput, intervals ...interface{}) {
	Eventually(func() (bool, error) {
		if err := input.Getter.Get(ctx, client.ObjectKeyFromObject(input.Machine), input.Machine); err != nil {
			return false, errors.Wrapf(err, "failed to get machine")
		}

		return input.Checker(input.Machine, input.Condition), nil
	}, intervals...).Should(BeTrue(), framework.PrettyPrint(input.Machine)+"\n")
}

// WaitForClusterToUpgradeInput is the input for WaitForClusterToUpgrade.
type WaitForClusterToUpgradeInput struct {
	Reader              framework.GetLister
	ControlPlane        *controlplanev1.RKE2ControlPlane
	MachineDeployments  []*clusterv1.MachineDeployment
	VersionAfterUpgrade string
}

// WaitForClusterToUpgrade will wait for a cluster to be upgraded.
func WaitForClusterToUpgrade(ctx context.Context, input WaitForClusterToUpgradeInput, intervals ...interface{}) {
	By("Waiting for machines to update")

	Eventually(func() error {
		cp := input.ControlPlane.DeepCopy()
		if err := input.Reader.Get(ctx, client.ObjectKeyFromObject(input.ControlPlane), cp); err != nil {
			return fmt.Errorf("failed to get control plane: %w", err)
		}

		updatedDeployments := []*clusterv1.MachineDeployment{}
		for _, md := range input.MachineDeployments {
			copy := &clusterv1.MachineDeployment{}
			if err := input.Reader.Get(ctx, client.ObjectKeyFromObject(md), copy); client.IgnoreNotFound(err) != nil {
				return fmt.Errorf("failed to get updated machine deployment: %w", err)
			}

			updatedDeployments = append(updatedDeployments, copy)
		}

		machineList := &clusterv1.MachineList{}
		if err := input.Reader.List(ctx, machineList); err != nil {
			return fmt.Errorf("failed to list machines: %w", err)
		}

		for _, machine := range machineList.Items {
			expectedVersion := input.VersionAfterUpgrade + "+rke2r1"
			if machine.Spec.Version == "" || machine.Spec.Version != expectedVersion {
				return fmt.Errorf("Expected machine version to match %s, got %v", expectedVersion, machine.Spec.Version)
			}
		}

		ready := ptr.Equal(cp.Status.ReadyReplicas, cp.Status.Replicas)
		if !ready {
			return fmt.Errorf("Control plane is not ready: %d ready from %d", cp.Status.ReadyReplicas, cp.Status.Replicas)
		}

		expected := cp.Spec.Replicas != nil && ptr.Equal(cp.Spec.Replicas, cp.Status.Replicas)
		if !expected {
			return fmt.Errorf("Control plane is not scaled: %d replicas from %d", cp.Spec.Replicas, cp.Status.Replicas)
		}

		for _, md := range updatedDeployments {
			if md.Spec.Replicas == nil || !ptr.Equal(md.Spec.Replicas, md.Status.ReadyReplicas) {
				return fmt.Errorf("Not all machine deployments are updated yet expected %v!=%d", md.Spec.Replicas, md.Status.ReadyReplicas)
			}
		}

		return nil
	}, intervals...).Should(Succeed())
}

// WaitForClusterReadyInput is the input type for WaitForClusterReady.
type WaitForClusterReadyInput struct {
	Getter    framework.Getter
	Name      string
	Namespace string
}

// WaitForClusterReady will wait for a Cluster to be Ready.
func WaitForClusterReady(ctx context.Context, input WaitForClusterReadyInput, intervals ...interface{}) {
	By("Waiting for Cluster to be Ready")

	Eventually(func() error {
		cluster := &clusterv1.Cluster{}
		key := types.NamespacedName{Name: input.Name, Namespace: input.Namespace}

		if err := input.Getter.Get(ctx, key, cluster); err != nil {
			return fmt.Errorf("getting Cluster %s/%s: %w", input.Namespace, input.Name, err)
		}
		// fmt.Println("#### Cluster object:", framework.PrettyPrint(cluster))

		readyCondition := conditions.Get(cluster, clusterv1.ReadyCondition)
		if readyCondition == nil {
			fmt.Println("#### Cluster condition", clusterv1.ReadyCondition, "not found")
			return fmt.Errorf("Cluster Ready condition is not found")
		}
		fmt.Println("#### Cluster ready condition:", clusterv1.ReadyCondition, "type:", readyCondition.Type, "status:", readyCondition.Status, "reason:", readyCondition.Reason)

		switch readyCondition.Status {
		case metav1.ConditionTrue:
			// Cluster is ready
			return nil
		case metav1.ConditionFalse:
			return fmt.Errorf("Cluster is not Ready")
		default:
			return fmt.Errorf("Cluster Ready condition is unknown")
		}
	}, intervals...).Should(Succeed())
}

// WaitForClusterReady will wait for a Cluster to be Ready.
func WaitForClusterReadyV1Beta1(ctx context.Context, input WaitForClusterReadyInput, intervals ...interface{}) {
	Byf("Waiting for v1beta1 Cluster %s/%s to be Ready", input.Namespace, input.Name)

	Eventually(func() error {
		cluster := &clusterv1beta1.Cluster{}
		key := types.NamespacedName{Name: input.Name, Namespace: input.Namespace}

		if err := input.Getter.Get(ctx, key, cluster); err != nil {
			return fmt.Errorf("getting Cluster %s/%s: %w", input.Namespace, input.Name, err)
		}

		for _, condition := range cluster.Status.Conditions {
			if condition.Type == clusterv1beta1.ConditionType("Ready") {
				switch condition.Status {
				case corev1.ConditionTrue:
					// Cluster is ready
					return nil
				case corev1.ConditionFalse:
					return fmt.Errorf("Cluster is not Ready")
				default:
					return fmt.Errorf("Cluster Ready condition is unknown")
				}
			}
		}
		return fmt.Errorf("Cluster Ready condition is not found")
	}, intervals...).Should(Succeed())
}

// VerifyDockerMachineTemplateOwner is the input type for VerifyDockerMachineTemplateOwner.
type VerifyDockerMachineTemplateOwnerInput struct {
	Lister      framework.Lister
	ClusterName string
	Namespace   string
}

// VerifyDockerMachineTemplateOwner ensures all DockerMachineTemplates used by the Cluster, are also owned by it.
// Note that for this test to work we manually label the DockerMachineTemplates with 'cluster.x-k8s.io/cluster-name: ${CLUSTER_NAME}'.
// DockerMachineTemplates cloned from ClusterClasses' templates, are automatically labeled.
func VerifyDockerMachineTemplateOwner(ctx context.Context, input VerifyDockerMachineTemplateOwnerInput) {
	By("Verifying DockerMachineTemplates are owned by Cluster " + input.Namespace + "/" + input.ClusterName)
	infraMachineTemplates := &dockerv1.DockerMachineTemplateList{}
	Expect(input.Lister.List(ctx, infraMachineTemplates,
		client.MatchingLabels{clusterv1.ClusterNameLabel: input.ClusterName},
		client.InNamespace(input.Namespace),
	)).Should(Succeed())
	Expect(infraMachineTemplates.Items).ShouldNot(BeEmpty())
	for _, infraMachineTemplate := range infraMachineTemplates.Items {
		Expect(infraMachineTemplate.OwnerReferences).ShouldNot(BeEmpty())
		Expect(infraMachineTemplate.OwnerReferences[0].Kind).Should(Equal("Cluster"))
		Expect(infraMachineTemplate.OwnerReferences[0].Name).Should(Equal(input.ClusterName))
	}
}

// EnsureNoMachineRollout will consistently verify that Machine rollout did not happen, by comparing an machine list.
func EnsureNoMachineRollout(ctx context.Context, input GetMachinesByClusterInput, machineNames []string) {
	By("Verifying machine rollout did not happen")
	Consistently(func() error {
		updatedMachinesNames := GetMachineNamesByCluster(ctx, input)
		if len(updatedMachinesNames) == 0 {
			return fmt.Errorf("There must be at least one Machine after provider upgrade")
		}
		sameMachines, err := ContainElements(machineNames).Match(updatedMachinesNames)
		if err != nil {
			return fmt.Errorf("matching machines: %w", err)
		}
		if !sameMachines {
			fmt.Printf("Pre-upgrade machines: [%s]\n", strings.Join(machineNames, ","))
			fmt.Printf("Post-upgrade machines: [%s]\n", strings.Join(updatedMachinesNames, ","))
			return fmt.Errorf("Machines should not have been rolled out after provider upgrade")
		}
		if len(updatedMachinesNames) != len(machineNames) {
			return fmt.Errorf("Number of Machines '%d' should match after provider upgrade '%d'", len(machineNames), len(updatedMachinesNames))
		}
		return nil
	}).WithTimeout(2 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

type WaitForAllMachinesRunningWithVersionInput struct {
	Reader      framework.GetLister
	Version     string
	ClusterName string
	Namespace   string
}

func WaitForAllMachinesRunningWithVersion(ctx context.Context, input WaitForAllMachinesRunningWithVersionInput, intervals ...any) {
	Byf("Waiting for all machines to be Running with version %s", input.Version)
	Eventually(func() bool {
		opts := []client.ListOption{
			client.InNamespace(input.Namespace),
			client.MatchingLabels{
				clusterv1.ClusterNameLabel: input.ClusterName,
			},
		}

		machineList := &clusterv1.MachineList{}
		Eventually(func() error {
			return input.Reader.List(ctx, machineList, opts...)
		}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list Machine objects for Cluster %s", klog.KRef(input.Namespace, input.ClusterName))

		for _, machine := range machineList.Items {
			if machine.Status.Phase != "Running" ||
				machine.Spec.Version == "" || !strings.Contains(machine.Spec.Version, input.Version) {
				return false
			}
		}
		return true
	}, intervals...).Should(BeTrue())
}

// StoreImagesInput is the input for StoreImages.
type StoreImagesInput struct {
	// Directory where to save the images
	Directory string

	// Images to be saved
	Images []clusterctl.ContainerImage
}

// StoreImages will save all images to a path.
// This can be used in the move/pivot test to pre-load images to the RKE2 workload cluster.
func StoreImages(ctx context.Context, input StoreImagesInput) error {
	Expect(input.Directory).ShouldNot(BeNil(), "context is required")
	Expect(input.Images).ShouldNot(BeEmpty(), "images list can't be empty")
	Expect(os.MkdirAll(input.Directory, os.ModePerm)).Should(Succeed())

	containerRuntime, err := container.NewDockerClient()
	if err != nil {
		return fmt.Errorf("accessing container runtime: %w", err)
	}

	for _, image := range input.Images {
		// in the nominal E2E scenario images have been locally built and added to cache
		exists, err := containerRuntime.ImageExistsLocally(ctx, image.Name)
		if err != nil {
			return fmt.Errorf("listing local image %s: %w", image.Name, err)
		}
		// in some scenarios we refer to a real reference image which may not have been pre-downloaded
		if !exists {
			GinkgoWriter.Printf("Image %s not present in local container image cache, will pull\n", image.Name)
			err := containerRuntime.PullContainerImage(ctx, image.Name)
			if err != nil {
				return fmt.Errorf("pulling image %s: %w", image.Name, err)
			}
		} else {
			GinkgoWriter.Printf("Image %s is present in local container image cache\n", image.Name)
		}

		// save the image
		parts := strings.Split(image.Name, ":")
		Expect(len(parts)).Should(BeNumerically(">", 1), "Image should have tag")
		replacer := strings.NewReplacer(".", "-", "/", "_")
		imageTarPath := filepath.Join(input.Directory, fmt.Sprintf("%s.tar", replacer.Replace(parts[0])))

		GinkgoWriter.Printf("Saving image to: %s\n", imageTarPath)
		err = containerRuntime.SaveContainerImage(ctx, image.Name, imageTarPath)
		if err != nil {
			return fmt.Errorf("saving image %s to %s: %w", image.Name, imageTarPath, err)
		}
	}

	return nil
}

// setDefaults sets the default values for ApplyCustomClusterTemplateAndWaitInput if not set.
// Currently, we set the default ControlPlaneWaiters here, which are implemented for RKE2ControlPlane.
func setDefaults(input *ApplyCustomClusterTemplateAndWaitInput) {
	if input.WaitForControlPlaneInitialized == nil {
		input.WaitForControlPlaneInitialized = func(ctx context.Context, input ApplyCustomClusterTemplateAndWaitInput, result *ApplyCustomClusterTemplateAndWaitResult) {
			result.ControlPlane = DiscoveryAndWaitForRKE2ControlPlaneInitialized(ctx, DiscoveryAndWaitForRKE2ControlPlaneInitializedInput{
				Lister:  input.ClusterProxy.GetClient(),
				Cluster: result.Cluster,
			}, input.WaitForControlPlaneIntervals...)
		}
	}

	if input.WaitForControlPlaneMachinesReady == nil {
		input.WaitForControlPlaneMachinesReady = func(ctx context.Context, input ApplyCustomClusterTemplateAndWaitInput, result *ApplyCustomClusterTemplateAndWaitResult) {
			WaitForControlPlaneAndMachinesReady(ctx, WaitForControlPlaneAndMachinesReadyInput{
				GetLister:    input.ClusterProxy.GetClient(),
				Cluster:      result.Cluster,
				ControlPlane: result.ControlPlane,
			}, input.WaitForControlPlaneIntervals...)
		}
	}
}

var secrets = []string{}

func CollectArtifacts(ctx context.Context, kubeconfigPath, name string, args ...string) error {
	if kubeconfigPath == "" {
		return fmt.Errorf("Unable to collect artifacts: kubeconfig path is empty")
	}

	aargs := append([]string{"crust-gather", "collect", "--kubeconfig", kubeconfigPath, "-v", "ERROR", "-f", name}, args...)
	for _, secret := range secrets {
		aargs = append(aargs, "-s", secret)
	}

	cmd := exec.Command("kubectl", aargs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Minute

	fmt.Printf("Running kubectl %s\n", strings.Join(aargs, " "))
	err := cmd.Run()
	fmt.Printf("stderr:\n%s\n", string(stderr.String()))
	fmt.Printf("stdout:\n%s\n", string(stdout.String()))
	return err
}

// Apply wraps `kubectl apply ...` and prints the output so we can see what gets applied to the cluster.
func Apply(ctx context.Context, clusterProxy framework.ClusterProxy, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Apply")
	Expect(resources).NotTo(BeNil(), "resources is required for Apply")

	if err := KubectlApply(ctx, clusterProxy.GetKubeconfigPath(), resources, args...); err != nil {

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return errors.New(fmt.Sprintf("%s: stderr: %s", err.Error(), exitErr.Stderr))
		}
	}
	return nil
}

// KubectlApply shells out to kubectl apply.
func KubectlApply(ctx context.Context, kubeconfigPath string, resources []byte, args ...string) error {
	aargs := append([]string{"apply", "--kubeconfig", kubeconfigPath, "-f", "-"}, args...)
	rbytes := bytes.NewReader(resources)
	applyCmd := NewCommand(
		WithCommand(kubectlPath()),
		WithArgs(aargs...),
		WithStdin(rbytes),
	)

	fmt.Printf("Running kubectl %s\n", strings.Join(aargs, " "))
	stdout, stderr, err := applyCmd.Run(ctx)
	if len(stderr) > 0 {
		fmt.Printf("stderr:\n%s\n", string(stderr))
	}
	if len(stdout) > 0 {
		fmt.Printf("stdout:\n%s\n", string(stdout))
	}
	return err
}

// KubectlWait shells out to kubectl wait.
func KubectlWait(ctx context.Context, kubeconfigPath string, args ...string) error {
	wargs := append([]string{"wait", "--kubeconfig", kubeconfigPath}, args...)
	wait := NewCommand(
		WithCommand(kubectlPath()),
		WithArgs(wargs...),
	)
	_, stderr, err := wait.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return err
	}
	return nil
}

func kubectlPath() string {
	if kubectlPath, ok := os.LookupEnv("CAPI_KUBECTL_PATH"); ok {
		return kubectlPath
	}
	return "kubectl"
}

type Command struct {
	Cmd   string
	Args  []string
	Stdin io.Reader
}

// Option is a functional option type that modifies a Command.
type Option func(*Command)

// NewCommand returns a configured Command.
func NewCommand(opts ...Option) *Command {
	cmd := &Command{
		Stdin: nil,
	}
	for _, option := range opts {
		option(cmd)
	}
	return cmd
}

// WithStdin sets up the command to read from this io.Reader.
func WithStdin(stdin io.Reader) Option {
	return func(cmd *Command) {
		cmd.Stdin = stdin
	}
}

// WithCommand defines the command to run such as `kubectl` or `kind`.
func WithCommand(command string) Option {
	return func(cmd *Command) {
		cmd.Cmd = command
	}
}

// WithArgs sets the arguments for the command such as `get pods -n kube-system` to the command `kubectl`.
func WithArgs(args ...string) Option {
	return func(cmd *Command) {
		cmd.Args = args
	}
}

// Run executes the command and returns stdout, stderr and the error if there is any.
func (c *Command) Run(ctx context.Context) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, c.Cmd, c.Args...) //nolint:gosec
	if c.Stdin != nil {
		cmd.Stdin = c.Stdin
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, errors.WithStack(err)
	}
	output, err := io.ReadAll(stdout)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	errout, err := io.ReadAll(stderr)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if err := cmd.Wait(); err != nil {
		return output, errout, errors.WithStack(err)
	}
	return output, errout, nil
}
