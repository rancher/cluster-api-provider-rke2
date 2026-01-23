/*
Copyright 2022 SUSE.

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

package rke2

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/rest"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/etcd"
)

const (
	labelNodeRoleControlPlane = "node-role.kubernetes.io/control-plane"
	remoteEtcdTimeout         = 30 * time.Second
	etcdDialTimeout           = 10 * time.Second
	etcdCallTimeout           = 15 * time.Second
	rke2ServingSecretKey      = "rke2-serving" //nolint: gosec
)

// WorkloadCluster defines all behaviors necessary to upgrade kubernetes on a workload cluster.
type WorkloadCluster interface {
	// Basic health and status checks.
	InitWorkload(ctx context.Context, controlPlane *ControlPlane) error
	UpdateNodeMetadata(ctx context.Context, controlPlane *ControlPlane) error

	ClusterStatus(ctx context.Context) ClusterStatus
	UpdateAgentConditions(controlPlane *ControlPlane)
	UpdateEtcdConditions(controlPlane *ControlPlane)
	// Upgrade related tasks.

	//	AllowBootstrapTokensToGetNodes(ctx context.Context) error

	// State recovery tasks.
	RemoveEtcdMemberForMachine(ctx context.Context, machine *clusterv1.Machine) error
	IsEtcdMemberSafelyRemovedForMachine(ctx context.Context, machine *clusterv1.Machine) (bool, error)
	ForwardEtcdLeadership(ctx context.Context, machine *clusterv1.Machine, leaderCandidate *clusterv1.Machine) error
	EtcdMembers(ctx context.Context) ([]string, error)

	// Common tasks.
	ApplyLabelOnNode(ctx context.Context, machine *clusterv1.Machine, label, value string) error
}

// Workload defines operations on workload clusters.
type Workload struct {
	ctrlclient.Client

	Nodes               map[string]*corev1.Node
	nodePatchHelpers    map[string]*patch.Helper
	etcdClientGenerator etcd.ClientFor
}

// NewWorkload is creating a new ClusterWorkload instance.
func (m *Management) NewWorkload(
	ctx context.Context,
	cl ctrlclient.Client,
	restConfig *rest.Config,
	clusterKey ctrlclient.ObjectKey,
) (*Workload, error) {
	workload := &Workload{
		Client:           cl,
		Nodes:            map[string]*corev1.Node{},
		nodePatchHelpers: map[string]*patch.Helper{},
	}

	restConfig = rest.CopyConfig(restConfig)
	restConfig.Timeout = remoteEtcdTimeout

	// Retrieves the etcd CA key Pair
	etcdKeyPair, err := m.getEtcdCAKeyPair(ctx, m.SecretCachingClient, clusterKey)
	if ctrlclient.IgnoreNotFound(err) != nil {
		return nil, err
	} else if apierrors.IsNotFound(err) {
		etcdKeyPair, err = m.getEtcdCAKeyPair(ctx, m.Client, clusterKey)
		if ctrlclient.IgnoreNotFound(err) != nil {
			return nil, err
		}
	}

	if apierrors.IsNotFound(err) || etcdKeyPair == nil {
		log.FromContext(ctx).Info("Cluster does not provide etcd certificates for creating child etcd ctrlclient.")

		return workload, nil
	}

	clientCert, err := tls.X509KeyPair(etcdKeyPair.Cert, etcdKeyPair.Key)
	if err != nil {
		return nil, err
	}

	if !strings.Contains(string(etcdKeyPair.Key), "EC PRIVATE KEY") {
		clientKey, err := m.ClusterCache.GetClientCertificatePrivateKey(ctx, clusterKey)
		if err != nil {
			return nil, err
		}

		if clientKey == nil {
			return nil, errors.New("client key is not populated yet, requeuing")
		}

		clientCert, err = generateClientCert(etcdKeyPair.Cert, etcdKeyPair.Key, clientKey)
		if err != nil {
			return nil, err
		}
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(etcdKeyPair.Cert)
	tlsConfig := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{clientCert},
		MinVersion:   tls.VersionTLS12,
	}
	tlsConfig.InsecureSkipVerify = true
	workload.etcdClientGenerator = etcd.NewClientGenerator(restConfig, tlsConfig, etcdDialTimeout, etcdCallTimeout)

	return workload, nil
}

// InitWorkload prepares workload for evaluating status conditions.
func (w *Workload) InitWorkload(ctx context.Context, cp *ControlPlane) error {
	nodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		v1beta1conditions.MarkUnknown(
			cp.RCP,
			controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition,
			controlplanev1.ControlPlaneComponentsInspectionFailedV1Beta1Reason, "Failed to list nodes which are hosting control plane components")
		conditions.Set(cp.RCP, metav1.Condition{
			Type:    controlplanev1.RKE2ControlPlaneControlPlaneComponentsHealthyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.RKE2ControlPlaneControlPlaneComponentsInspectionFailedReason,
			Message: "Failed to list nodes hosting control plane " + cp.RCP.Name,
		})

		return err
	}

	for _, node := range nodes.Items {
		nodeCopy := node
		w.Nodes[node.Name] = &nodeCopy
	}

	for _, node := range w.Nodes {
		patchHelper, err := patch.NewHelper(node, w.Client)
		if err != nil {
			v1beta1conditions.MarkUnknown(
				cp.RCP,
				controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition,
				controlplanev1.ControlPlaneComponentsInspectionFailedV1Beta1Reason, "Failed to create patch helpers for control plane nodes")
			conditions.Set(cp.RCP, metav1.Condition{
				Type:    controlplanev1.RKE2ControlPlaneControlPlaneComponentsHealthyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.RKE2ControlPlaneControlPlaneComponentsInspectionFailedReason,
				Message: fmt.Sprintf("Failed to create patch helper for control plane %s nodes", cp.RCP.Name),
			})

			return err
		}

		w.nodePatchHelpers[node.Name] = patchHelper
	}

	return nil
}

// ClusterStatus holds stats information about the cluster.
type ClusterStatus struct {
	// Nodes are a total count of nodes
	Nodes int32
	// ReadyNodes are the count of nodes that are reporting ready
	ReadyNodes int32
	// HasRKE2ServingSecret will be true if the rke2-serving secret has been uploaded, false otherwise.
	HasRKE2ServingSecret bool
}

// PatchNodes patches the nodes in the workload cluster.
func (w *Workload) PatchNodes(ctx context.Context, cp *ControlPlane) error {
	errList := []error{}

	for i := range w.Nodes {
		node := w.Nodes[i]
		machine, found := cp.Machines[node.Name]

		if !found {
			for _, m := range cp.Machines {
				if m.Status.NodeRef.IsDefined() && m.Status.NodeRef.Name == node.Name {
					machine = m

					break
				}
			}

			if machine == nil {
				continue
			}
		}

		if helper, ok := w.nodePatchHelpers[node.Name]; ok {
			if err := helper.Patch(ctx, node); err != nil {
				v1beta1conditions.MarkUnknown(
					machine,
					controlplanev1.NodeMetadataUpToDateV1Beta1Condition,
					controlplanev1.NodePatchFailedV1Beta1Reason, "%s", err.Error())

				conditions.Set(machine, metav1.Condition{
					Type:    controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.RKE2ControlPlaneNodePatchFailedReason,
					Message: "Failed to patch node %s" + node.Name,
				})

				errList = append(errList, err)
			}

			continue
		}

		errList = append(errList, errors.Errorf("failed to get patch helper for node %s", node.Name))
	}

	return kerrors.NewAggregate(errList)
}

// ClusterStatus returns the status of the cluster.
func (w *Workload) ClusterStatus(ctx context.Context) ClusterStatus {
	status := ClusterStatus{}

	// count the control plane nodes
	for _, node := range w.Nodes {
		nodeCopy := node
		status.Nodes++

		if util.IsNodeReady(nodeCopy) {
			status.ReadyNodes++
		}
	}

	// find the rke2-serving secret
	key := ctrlclient.ObjectKey{
		Name:      rke2ServingSecretKey,
		Namespace: metav1.NamespaceSystem,
	}
	err := w.Get(ctx, key, &corev1.Secret{})
	// In case of error we do assume the control plane is not initialized yet.
	if err != nil {
		logger := log.FromContext(ctx)
		logger.Info("Control Plane does not seem to be initialized yet.", "reason", err.Error())
	}

	status.HasRKE2ServingSecret = err == nil

	return status
}

func hasProvisioningMachine(machines collections.Machines) bool {
	for _, machine := range machines {
		if !machine.Status.NodeRef.IsDefined() {
			return true
		}
	}

	return false
}

// UpdateNodeMetadata is responsible for populating node metadata after
// it is referenced from machine object.
func (w *Workload) UpdateNodeMetadata(ctx context.Context, controlPlane *ControlPlane) error {
	for nodeName, machine := range controlPlane.Machines {
		if !machine.Spec.Bootstrap.ConfigRef.IsDefined() {
			continue
		}

		if machine.Status.NodeRef.IsDefined() {
			nodeName = machine.Status.NodeRef.Name
		}

		v1beta1conditions.MarkTrue(machine, controlplanev1.NodeMetadataUpToDateV1Beta1Condition)
		conditions.Set(machine, metav1.Condition{
			Type:   controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition,
			Status: metav1.ConditionTrue,
			Reason: controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateReason,
		})

		node, nodeFound := w.Nodes[nodeName]
		if !nodeFound {
			v1beta1conditions.MarkUnknown(
				machine,
				controlplanev1.NodeMetadataUpToDateV1Beta1Condition,
				controlplanev1.NodePatchFailedV1Beta1Reason, "associated node not found")
			conditions.Set(machine, metav1.Condition{
				Type:    controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.RKE2ControlPlaneNodePatchFailedReason,
				Message: fmt.Sprintf("node %s not found", nodeName),
			})

			continue
		} else if name, ok := node.Annotations[clusterv1.MachineAnnotation]; !ok || name != machine.Name {
			v1beta1conditions.MarkUnknown(
				machine,
				controlplanev1.NodeMetadataUpToDateV1Beta1Condition,
				controlplanev1.NodePatchFailedV1Beta1Reason,
				"node object is missing %s annotation",
				clusterv1.MachineAnnotation)
			conditions.Set(machine, metav1.Condition{
				Type:    controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.RKE2ControlPlaneNodePatchFailedReason,
				Message: fmt.Sprintf("node %s is missing %s annotation", nodeName, clusterv1.MachineAnnotation),
			})

			continue
		}

		rkeConfig, found := controlPlane.Rke2Configs[machine.Name]
		if !found {
			v1beta1conditions.MarkUnknown(
				machine,
				controlplanev1.NodeMetadataUpToDateV1Beta1Condition,
				controlplanev1.NodePatchFailedV1Beta1Reason, "associated RKE2 config not found")
			conditions.Set(machine, metav1.Condition{
				Type:    controlplanev1.RKE2ControlPlaneNodeMetadataUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.RKE2ControlPlaneNodePatchFailedReason,
				Message: fmt.Sprintf("node %s's RKE2Config not found", nodeName),
			})

			continue
		}

		annotations.AddAnnotations(node, rkeConfig.Spec.AgentConfig.NodeAnnotations)
	}

	return w.PatchNodes(ctx, controlPlane)
}

// ApplyLabelOnNode applies a label key and value to the Node associated to the Machine, if any.
func (w *Workload) ApplyLabelOnNode(ctx context.Context, machine *clusterv1.Machine, key, value string) error {
	logger := log.FromContext(ctx)
	if machine == nil || !machine.Status.NodeRef.IsDefined() {
		// Nothing to do, no node for Machine
		logger.Info("Nothing to do, no node for Machine")

		return nil
	}

	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return fmt.Errorf("getting control plane nodes: %w", err)
	}

	for _, node := range controlPlaneNodes.Items {
		if node.Name == machine.Status.NodeRef.Name {
			if node.Labels == nil {
				node.Labels = map[string]string{}
			}

			node.Labels[key] = value

			if helper, ok := w.nodePatchHelpers[node.Name]; ok {
				if err := helper.Patch(ctx, &node); err != nil {
					return fmt.Errorf("patching node %s with %s label: %w", node.Name, key, err)
				}
			} else {
				return fmt.Errorf("could not find patch helper for node %s", node.Name)
			}

			break
		}
	}

	logger.Info(fmt.Sprintf("Applied label %s on node %s", key, machine.Status.NodeRef.Name))

	return nil
}

func (w *Workload) getControlPlaneNodes(ctx context.Context) (*corev1.NodeList, error) {
	nodes := &corev1.NodeList{}
	labels := map[string]string{
		labelNodeRoleControlPlane: "true",
	}

	if err := w.List(ctx, nodes, ctrlclient.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return nodes, nil
}
