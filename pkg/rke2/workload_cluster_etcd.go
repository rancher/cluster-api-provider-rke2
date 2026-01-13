/*
Copyright 2020 The Kubernetes Authors.

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
	"fmt"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	etcdutil "github.com/rancher/cluster-api-provider-rke2/pkg/etcd/util"
)

const (
	// EtcdNodeRemoveAnnotation is a Node annotation used to notify RKE2 of removing the etcd member for a node.
	EtcdNodeRemoveAnnotation = "etcd.rke2.cattle.io/remove"
	// EtcdNodeRemovedNodeNameAnnotation is a Node annotation used by RKE2 to notify the etcd member has been successfully removed.
	EtcdNodeRemovedNodeNameAnnotation = "etcd.rke2.cattle.io/removed-node-name"
)

// RemoveEtcdMemberForMachine removes the etcd member from the target cluster's etcd cluster.
// Removing the last remaining member of the cluster is not supported.
func (w *Workload) RemoveEtcdMemberForMachine(ctx context.Context, machine *clusterv1.Machine) error {
	if machine == nil || !machine.Status.NodeRef.IsDefined() {
		// Nothing to do, no node for Machine
		return nil
	}

	return w.removeMemberForNode(ctx, machine.Status.NodeRef.Name)
}

func (w *Workload) removeMemberForNode(ctx context.Context, name string) error {
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return fmt.Errorf("getting control plane nodes: %w", err)
	}

	for _, node := range controlPlaneNodes.Items {
		if node.Name == name {
			if node.Annotations == nil {
				node.Annotations = map[string]string{}
			}

			node.Annotations[EtcdNodeRemoveAnnotation] = "true"

			if helper, ok := w.nodePatchHelpers[node.Name]; ok {
				if err := helper.Patch(ctx, &node); err != nil {
					return fmt.Errorf("patching node %s with etcd remove annotation: %w", node.Name, err)
				}
			} else {
				return fmt.Errorf("could not find patch helper for node %s", node.Name)
			}

			break
		}
	}

	log.FromContext(ctx).Info("Removed member: " + name)

	return nil
}

// IsEtcdMemberSafelyRemovedForMachine checks whether the node contains the `etcd.rke2.cattle.io/removed-node-name` annotation.
func (w *Workload) IsEtcdMemberSafelyRemovedForMachine(ctx context.Context, machine *clusterv1.Machine) (bool, error) {
	if machine == nil || !machine.Status.NodeRef.IsDefined() {
		// Nothing to do, no node for Machine
		return true, nil
	}

	return w.isMemberRemovedForNode(ctx, machine.Status.NodeRef.Name)
}

func (w *Workload) isMemberRemovedForNode(ctx context.Context, name string) (bool, error) {
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return false, fmt.Errorf("getting control plane nodes: %w", err)
	}

	for _, node := range controlPlaneNodes.Items {
		if node.Name == name {
			if node.Annotations == nil {
				return false, fmt.Errorf("node is missing the %s annotation", EtcdNodeRemoveAnnotation)
			}

			removedNodeName, found := node.Annotations[EtcdNodeRemovedNodeNameAnnotation]

			if !found {
				return false, nil
			}

			if removedNodeName != "" {
				log.FromContext(ctx).Info("Removed member: " + name)

				return true, nil
			}

			return false, nil
		}
	}

	return false, nil
}

// ForwardEtcdLeadership forwards etcd leadership to the first follower.
func (w *Workload) ForwardEtcdLeadership(ctx context.Context, machine *clusterv1.Machine, leaderCandidate *clusterv1.Machine) error {
	if machine == nil || !machine.Status.NodeRef.IsDefined() {
		return nil
	}

	if leaderCandidate == nil {
		return errors.New("leader candidate cannot be nil")
	}

	if !leaderCandidate.Status.NodeRef.IsDefined() {
		return errors.New("leader has no node reference")
	}

	// Return early for clusters without an etcd certificate secret
	if w.etcdClientGenerator == nil {
		return nil
	}

	nodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list control plane nodes")
	}

	nodeNames := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}

	etcdClient, err := w.etcdClientGenerator.ForLeader(ctx, nodeNames)
	if err != nil {
		return errors.Wrap(err, "failed to create etcd client")
	}
	defer etcdClient.Close()

	members, err := etcdClient.Members(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list etcd members using etcd client")
	}

	currentMember := etcdutil.MemberForName(members, machine.Status.NodeRef.Name)
	if currentMember == nil || currentMember.ID != etcdClient.LeaderID {
		// nothing to do, this is not the etcd leader
		return nil
	}

	// Move the leader to the provided candidate.
	nextLeader := etcdutil.MemberForName(members, leaderCandidate.Status.NodeRef.Name)
	if nextLeader == nil {
		return errors.Errorf("failed to get etcd member from node %q", leaderCandidate.Status.NodeRef.Name)
	}

	log.FromContext(ctx).Info(fmt.Sprintf("Moving leader from %s to %s", currentMember.Name, nextLeader.Name))

	if err := etcdClient.MoveLeader(ctx, nextLeader.ID); err != nil {
		return errors.Wrapf(err, "failed to move leader")
	}

	return nil
}

// EtcdMemberStatus contains status information for a single etcd member.
type EtcdMemberStatus struct {
	Name       string
	Responsive bool
}

// EtcdMembers returns the current set of members in an etcd cluster.
//
// NOTE: This methods uses control plane machines/nodes only to get in contact with etcd,
// but then it relies on etcd as ultimate source of truth for the list of members.
// This is intended to allow informed decisions on actions impacting etcd quorum.
func (w *Workload) EtcdMembers(ctx context.Context) ([]string, error) {
	// Return early for clusters without an etcd certificate secret
	if w.etcdClientGenerator == nil {
		return []string{}, nil
	}

	nodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list control plane nodes")
	}

	nodeNames := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}

	etcdClient, err := w.etcdClientGenerator.ForLeader(ctx, nodeNames)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create etcd client")
	}
	defer etcdClient.Close()

	members, err := etcdClient.Members(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list etcd members using etcd client")
	}

	names := []string{}
	for _, member := range members {
		// Convert etcd member to node name
		names = append(names, etcdutil.NodeNameFromMember(member))
	}

	return names, nil
}
