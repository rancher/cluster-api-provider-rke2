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
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
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
	Args                         []string // extra args to be used during `kubectl apply`
	PreWaitForCluster            func()
	PostMachinesProvisioned      func()
	ControlPlaneWaiters
}

// ApplyCustomClusterTemplateAndWaitInput is the input type for ApplyCustomClusterTemplateAndWait.
type ApplyCustomClusterTemplateAndWaitInput struct {
	ClusterProxy                 framework.ClusterProxy
	CustomTemplateYAML           []byte
	ClusterName                  string
	Namespace                    string
	Flavor                       string
	WaitForClusterIntervals      []interface{}
	WaitForControlPlaneIntervals []interface{}
	WaitForMachineDeployments    []interface{}
	Args                         []string // extra args to be used during `kubectl apply`
	PreWaitForCluster            func()
	PostMachinesProvisioned      func()
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
}

// ApplyCustomClusterTemplateAndWaitResult is the output type for ApplyCustomClusterTemplateAndWait.
type ApplyCustomClusterTemplateAndWaitResult struct {
	ClusterClass       *clusterv1.ClusterClass
	Cluster            *clusterv1.Cluster
	ControlPlane       *controlplanev1.RKE2ControlPlane
	MachineDeployments []*clusterv1.MachineDeployment
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

	Byf("Creating the RKE2 based workload cluster with name %q using the %q template (Kubernetes %s)",
		input.ConfigCluster.ClusterName, input.ConfigCluster.Flavor, input.ConfigCluster.KubernetesVersion)

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
		Flavor:                       input.ConfigCluster.Flavor,
		WaitForClusterIntervals:      input.WaitForClusterIntervals,
		WaitForControlPlaneIntervals: input.WaitForControlPlaneIntervals,
		WaitForMachineDeployments:    input.WaitForMachineDeployments,
		PreWaitForCluster:            input.PreWaitForCluster,
		PostMachinesProvisioned:      input.PostMachinesProvisioned,
		ControlPlaneWaiters:          input.ControlPlaneWaiters,
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
	Expect(result).ToNot(BeNil(), "Invalid argument. result can't be nil when calling ApplyClusterTemplateAndWait")

	Byf("Creating the workload cluster with name %q from the provided yaml", input.ClusterName)

	Byf("Applying the cluster template yaml of cluster %s", klog.KRef(input.Namespace, input.ClusterName))
	Eventually(func() error {
		return input.ClusterProxy.Apply(ctx, input.CustomTemplateYAML, input.Args...)
	}, input.WaitForClusterIntervals...).Should(Succeed(), "Failed to apply the cluster template")

	// Once we applied the cluster template we can run PreWaitForCluster.
	// Note: This can e.g. be used to verify the BeforeClusterCreate lifecycle hook is executed
	// and blocking correctly.
	if input.PreWaitForCluster != nil {
		Byf("Calling PreWaitForCluster for cluster %s", klog.KRef(input.Namespace, input.ClusterName))
		input.PreWaitForCluster()
	}

	Byf("Waiting for the cluster infrastructure of cluster %s to be provisioned", klog.KRef(input.Namespace, input.ClusterName))
	result.Cluster = framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
		Getter:    input.ClusterProxy.GetClient(),
		Namespace: input.Namespace,
		Name:      input.ClusterName,
	}, input.WaitForClusterIntervals...)

	if result.Cluster.Spec.Topology != nil {
		result.ClusterClass = framework.GetClusterClassByName(ctx, framework.GetClusterClassByNameInput{
			Getter:    input.ClusterProxy.GetClient(),
			Namespace: input.Namespace,
			Name:      result.Cluster.Spec.Topology.Class,
		})
	}

	Byf("Waiting for control plane of cluster %s to be initialized", klog.KRef(input.Namespace, input.ClusterName))
	input.WaitForControlPlaneInitialized(ctx, input, result)

	Byf("Waiting for control plane of cluster %s to be ready", klog.KRef(input.Namespace, input.ClusterName))
	input.WaitForControlPlaneMachinesReady(ctx, input, result)

	Byf("Waiting for the machine deployments of cluster %s to be provisioned", klog.KRef(input.Namespace, input.ClusterName))
	result.MachineDeployments = DiscoveryAndWaitForMachineDeployments(ctx, framework.DiscoveryAndWaitForMachineDeploymentsInput{
		Lister:  input.ClusterProxy.GetClient(),
		Cluster: result.Cluster,
	}, input.WaitForMachineDeployments...)

	if input.PostMachinesProvisioned != nil {
		Byf("Calling PostMachinesProvisioned for cluster %s", klog.KRef(input.Namespace, input.ClusterName))
		input.PostMachinesProvisioned()
	}
}

// DiscoveryAndWaitForMachineDeployments discovers the MachineDeployments existing in a cluster and waits for them to be ready (all the machine provisioned).
func DiscoveryAndWaitForMachineDeployments(ctx context.Context, input framework.DiscoveryAndWaitForMachineDeploymentsInput, intervals ...interface{}) []*clusterv1.MachineDeployment {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DiscoveryAndWaitForMachineDeployments")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling DiscoveryAndWaitForMachineDeployments")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling DiscoveryAndWaitForMachineDeployments")

	machineDeployments := framework.GetMachineDeploymentsByCluster(ctx, framework.GetMachineDeploymentsByClusterInput{
		Lister:      input.Lister,
		ClusterName: input.Cluster.Name,
		Namespace:   input.Cluster.Namespace,
	})

	for _, deployment := range machineDeployments {
		framework.AssertMachineDeploymentFailureDomains(ctx, framework.AssertMachineDeploymentFailureDomainsInput{
			Lister:            input.Lister,
			Cluster:           input.Cluster,
			MachineDeployment: deployment,
		})
	}

	Eventually(func(g Gomega) {
		machineDeployments := framework.GetMachineDeploymentsByCluster(ctx, framework.GetMachineDeploymentsByClusterInput{
			Lister:      input.Lister,
			ClusterName: input.Cluster.Name,
			Namespace:   input.Cluster.Namespace,
		})
		for _, deployment := range machineDeployments {
			g.Expect(*deployment.Spec.Replicas).To(BeEquivalentTo(deployment.Status.ReadyReplicas))
		}
	}, intervals...).Should(Succeed())

	return machineDeployments
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
		Byf("Waiting for the remaining control plane machines managed by %s to be provisioned", klog.KObj(input.ControlPlane))
		WaitForRKE2ControlPlaneMachinesToExist(ctx, WaitForRKE2ControlPlaneMachinesToExistInput{
			Lister:       input.GetLister,
			Cluster:      input.Cluster,
			ControlPlane: input.ControlPlane,
		}, intervals...)
	}

	Byf("Waiting for control plane %s to be ready (implies underlying nodes to be ready as well)", klog.KObj(input.ControlPlane))
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
			Byf("Failed to list the machines: %+v", err)
			return 0, err
		}
		count := 0
		for _, machine := range machineList.Items {
			if machine.Status.NodeRef != nil {
				count++
			}
		}
		return count, nil
	}, intervals...).Should(Equal(int(*input.ControlPlane.Spec.Replicas)), "Timed out waiting for %d control plane machines to exist", int(*input.ControlPlane.Spec.Replicas))
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
		updatedReplicas := controlplane.Status.UpdatedReplicas
		readyReplicas := controlplane.Status.ReadyReplicas
		unavailableReplicas := controlplane.Status.UnavailableReplicas

		// Control plane is still rolling out (and thus not ready) if:
		// * .spec.replicas, .status.replicas, .status.updatedReplicas,
		//   .status.readyReplicas are not equal and
		// * unavailableReplicas > 0
		if statusReplicas != *desiredReplicas ||
			updatedReplicas != *desiredReplicas ||
			readyReplicas != *desiredReplicas ||
			unavailableReplicas > 0 {
			return false, nil
		}

		return true, nil
	}, intervals...).Should(BeTrue(), framework.PrettyPrint(controlplane)+"\n")
}

type WaitForMachineConditionsInput struct {
	Getter    framework.Getter
	Machine   *clusterv1.Machine
	Checker   func(_ conditions.Getter, _ clusterv1.ConditionType) bool
	Condition clusterv1.ConditionType
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
			if machine.Spec.Version == nil || *machine.Spec.Version != expectedVersion {
				return fmt.Errorf("Expected machine version to match %s, got %v", expectedVersion, machine.Spec.Version)
			}
		}

		ready := cp.Status.ReadyReplicas == cp.Status.Replicas
		if !ready {
			return fmt.Errorf("Control plane is not ready: %d ready from %d", cp.Status.ReadyReplicas, cp.Status.Replicas)
		}

		expected := cp.Spec.Replicas != nil && *cp.Spec.Replicas == cp.Status.Replicas
		if !expected {
			return fmt.Errorf("Control plane is not scaled: %d replicas from %d", cp.Spec.Replicas, cp.Status.Replicas)
		}

		for _, md := range updatedDeployments {
			if md.Spec.Replicas == nil || *md.Spec.Replicas != md.Status.ReadyReplicas {
				return fmt.Errorf("Not all machine deployments are updated yet expected %v!=%d", md.Spec.Replicas, md.Status.ReadyReplicas)
			}
		}

		return nil
	}, intervals...).Should(Succeed())
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
	fmt.Printf("stderr:\n%s\n", string(stderr.Bytes()))
	fmt.Printf("stdout:\n%s\n", string(stdout.Bytes()))
	return err
}
