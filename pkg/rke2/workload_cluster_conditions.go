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

package rke2

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/cluster-api/util/conditions"
	clog "sigs.k8s.io/cluster-api/util/log"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
)

// UpdateEtcdConditions is responsible for updating machine conditions reflecting the status of all the etcd members.
// This operation is best effort, in the sense that in case of problems in retrieving member status, it sets
// the condition to Unknown state without returning any error.
func (w *Workload) UpdateEtcdConditions(controlPlane *ControlPlane) {
	w.updateManagedEtcdConditions(controlPlane)
}

func (w *Workload) updateManagedEtcdConditions(controlPlane *ControlPlane) {
	allMachineEtcdConditions := []string{
		controlplanev1.RKE2ControlPlaneMachineEtcdMemberHealthyCondition,
	}

	// Track errors for orphan nodes (nodes without corresponding machines).
	var rcpErrors []string

	// NOTE: This methods uses control plane nodes only to get in contact with etcd but then it relies on etcd
	// as ultimate source of truth for the list of members and for their health.
	for k := range w.Nodes {
		node := w.Nodes[k]

		machine, found := controlPlane.Machines[node.Name]
		if !found {
			// If there are machines still provisioning there is the chance that a node might be linked to a machine soon,
			// otherwise report the error at RCP level given that there is no machine to report on.
			if hasProvisioningMachine(controlPlane.Machines) {
				continue
			}

			for _, m := range controlPlane.Machines {
				if m.Status.NodeRef.IsDefined() && m.Status.NodeRef.Name == node.Name {
					machine = m

					break
				}
			}

			if machine == nil {
				// Node exists but no machine found - this is an orphan node.
				rcpErrors = append(rcpErrors, fmt.Sprintf("Control plane node %s does not have a corresponding machine", node.Name))

				continue
			}
		}

		// If the machine is deleting, report all the conditions as deleting
		if !machine.DeletionTimestamp.IsZero() {
			conditions.Set(machine, metav1.Condition{
				Type:    controlplanev1.RKE2ControlPlaneMachineEtcdMemberHealthyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.RKE2ControlPlaneMachineEtcdMemberHealthyReason,
				Message: fmt.Sprintf("Machine %s is deleting", machine.Name),
			})

			continue
		}

		conditions.Set(machine, metav1.Condition{
			Type:    controlplanev1.RKE2ControlPlaneMachineEtcdMemberHealthyCondition,
			Status:  metav1.ConditionTrue,
			Reason:  controlplanev1.RKE2ControlPlaneMachineEtcdMemberHealthyReason,
			Message: "",
		})
	}

	// Second pass: Handle machines without corresponding nodes (still provisioning or node missing).
	for i := range controlPlane.Machines {
		machine := controlPlane.Machines[i]

		// Skip machines that already have their conditions set in the first pass (they have a corresponding node).
		if machine.Status.NodeRef.IsDefined() {
			if _, found := w.Nodes[machine.Status.NodeRef.Name]; found {
				continue
			}
		}

		// Machine doesn't have a node yet (either NodeRef not set or node not found).
		// If there are machines still provisioning, this might be a timing issue - set to Unknown.
		if hasProvisioningMachine(controlPlane.Machines) {
			for _, condition := range allMachineEtcdConditions {
				conditions.Set(machine, metav1.Condition{
					Type:    condition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.RKE2ControlPlaneMachineEtcdMemberInspectionFailedReason,
					Message: "Waiting for node to be provisioned",
				})
			}

			continue
		}

		// No machines provisioning but node still missing - this is an error.
		for _, condition := range allMachineEtcdConditions {
			conditions.Set(machine, metav1.Condition{
				Type:    condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.RKE2ControlPlaneMachineEtcdMemberHealthyReason,
				Message: "Node does not exist",
			})
		}
	}

	// Aggregate etcd member conditions from machines at RCP level.
	aggregateConditionsFromMachinesToRCP(aggregateConditionsFromMachinesToRCPInput{
		controlPlane:      controlPlane,
		machineConditions: allMachineEtcdConditions,
		rcpErrors:         rcpErrors,
		condition:         controlplanev1.RKE2ControlPlaneEtcdClusterHealthyCondition,
		falseReason:       controlplanev1.RKE2ControlPlaneEtcdClusterNotHealthyReason,
		unknownReason:     controlplanev1.RKE2ControlPlaneEtcdClusterHealthUnknownReason,
		trueReason:        controlplanev1.RKE2ControlPlaneEtcdClusterHealthyReason,
		note:              "etcd member",
	})
}

// UpdateAgentConditions is responsible for updating machine conditions reflecting the health status
// of all the RKE2 agents running on control plane nodes. The agent health is determined by checking
// if the corresponding Kubernetes node is in Ready state. This operation is best effort - in case
// of problems retrieving node status, it sets conditions to Unknown state without returning any error.
func (w *Workload) UpdateAgentConditions(controlPlane *ControlPlane) {
	allMachinePodConditions := []string{
		controlplanev1.RKE2ControlPlaneMachineAgentHealthyCondition,
	}

	// Track errors for orphan nodes (nodes without corresponding machines).
	var rcpErrors []string

	// First pass: Update conditions for machines that have corresponding nodes.
	for k := range w.Nodes {
		node := w.Nodes[k]

		// Search for the machine corresponding to the node.
		machine, found := controlPlane.Machines[node.Name]
		if !found {
			// If there are machines still provisioning, this might be a timing issue - skip for now.
			if hasProvisioningMachine(controlPlane.Machines) {
				continue
			}

			// Try to find machine by NodeRef if direct name lookup failed.
			for _, m := range controlPlane.Machines {
				if m.Status.NodeRef.IsDefined() && m.Status.NodeRef.Name == node.Name {
					machine = m

					break
				}
			}

			if machine == nil {
				// Node exists but no machine found - this is an orphan node.
				rcpErrors = append(rcpErrors, fmt.Sprintf("Control plane node %s does not have a corresponding machine", node.Name))

				continue
			}
		}

		// If the machine is deleting, report all the conditions as deleting
		if !machine.DeletionTimestamp.IsZero() {
			for _, condition := range allMachinePodConditions {
				conditions.Set(machine, metav1.Condition{
					Type:    condition,
					Status:  metav1.ConditionFalse,
					Reason:  controlplanev1.RKE2ControlPlaneMachinePodDeletingReason,
					Message: "Machine is deleting",
				})
			}

			continue
		}

		// If the node is Unreachable, information about static pods could be stale so set all conditions to unknown.
		if nodeHasUnreachableTaint(*node) {
			// NOTE: We are assuming unreachable as a temporary condition, leaving to MHC
			// the responsibility to determine if the node is unhealthy or not.
			for _, condition := range allMachinePodConditions {
				conditions.Set(machine, metav1.Condition{
					Type:    condition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.RKE2ControlPlaneMachinePodInspectionFailedReason,
					Message: fmt.Sprintf("Node %s is unreachable", node.Name),
				})
			}

			continue
		}

		// Node is reachable and machine is not deleting - set AgentHealthy based on node Ready condition.
		if isNodeReady(node) {
			conditions.Set(machine, metav1.Condition{
				Type:    controlplanev1.RKE2ControlPlaneMachineAgentHealthyCondition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.RKE2ControlPlaneMachineAgentHealthyReason,
				Message: "",
			})
		} else {
			conditions.Set(machine, metav1.Condition{
				Type:    controlplanev1.RKE2ControlPlaneMachineAgentHealthyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.RKE2ControlPlaneMachinePodFailedReason,
				Message: fmt.Sprintf("Node %s is not ready", node.Name),
			})
		}
	}

	// Second pass: Handle machines without corresponding nodes (still provisioning or node missing).
	for i := range controlPlane.Machines {
		machine := controlPlane.Machines[i]

		// Skip machines that already have their conditions set in the first pass (they have a corresponding node).
		if machine.Status.NodeRef.IsDefined() {
			if _, found := w.Nodes[machine.Status.NodeRef.Name]; found {
				continue
			}
		}

		// Machine doesn't have a node yet (either NodeRef not set or node not found).
		// If there are machines still provisioning, this might be a timing issue - set to Unknown.
		if hasProvisioningMachine(controlPlane.Machines) {
			for _, condition := range allMachinePodConditions {
				conditions.Set(machine, metav1.Condition{
					Type:    condition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.RKE2ControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for node to be provisioned",
				})
			}

			continue
		}

		// No machines provisioning but node still missing - this is an error.
		for _, condition := range allMachinePodConditions {
			conditions.Set(machine, metav1.Condition{
				Type:    condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.RKE2ControlPlaneMachinePodFailedReason,
				Message: "Node does not exist",
			})
		}
	}

	aggregateConditionsFromMachinesToRCP(aggregateConditionsFromMachinesToRCPInput{
		controlPlane:      controlPlane,
		machineConditions: allMachinePodConditions,
		rcpErrors:         rcpErrors,
		condition:         controlplanev1.RKE2ControlPlaneControlPlaneComponentsHealthyCondition,
		falseReason:       controlplanev1.RKE2ControlPlaneControlPlaneComponentsNotHealthyReason,
		unknownReason:     controlplanev1.RKE2ControlPlaneControlPlaneComponentsHealthUnknownReason,
		trueReason:        controlplanev1.RKE2ControlPlaneControlPlaneComponentsHealthyReason,
		note:              "control plane",
	})
}

// isNodeReady returns true if the node has the Ready condition set to True.
func isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

// nodeHasUnreachableTaint returns true if the node has the unreachable taint from the node controller.
func nodeHasUnreachableTaint(node corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == corev1.TaintNodeUnreachable && taint.Effect == corev1.TaintEffectNoExecute {
			return true
		}
	}

	return false
}

type aggregateConditionsFromMachinesToRCPInput struct {
	controlPlane      *ControlPlane
	machineConditions []string
	rcpErrors         []string
	condition         string
	trueReason        string
	unknownReason     string
	falseReason       string
	note              string
}

// aggregateConditionsFromMachinesToRCP aggregates a group of conditions from machines to RCP.
// NOTE: the aggregation is computed in way that is similar to how conditions.NewAggregateCondition works, but in this case the
// implementation is simpler/less flexible and it surfaces only issues & unknown conditions.
func aggregateConditionsFromMachinesToRCP(input aggregateConditionsFromMachinesToRCPInput) {
	// Aggregates machines for condition status.
	// NB. A machine could be assigned to many groups, but only the group with the highest severity will be reported.
	rcpMachinesWithErrors := sets.Set[string]{}
	rcpMachinesWithUnknown := sets.Set[string]{}
	rcpMachinesWithInfo := sets.Set[string]{}

	messageMap := map[string][]string{}

	for i := range input.controlPlane.Machines {
		machine := input.controlPlane.Machines[i]
		machineMessages := []string{}
		conditionCount := 0
		conditionMessages := sets.Set[string]{}

		for _, condition := range input.machineConditions {
			if machineCondition := conditions.Get(machine, condition); machineCondition != nil {
				conditionCount++

				conditionMessages.Insert(machineCondition.Message)

				switch machineCondition.Status {
				case metav1.ConditionTrue:
					rcpMachinesWithInfo.Insert(machine.Name)
				case metav1.ConditionFalse:
					rcpMachinesWithErrors.Insert(machine.Name)

					m := machineCondition.Message

					if m == "" {
						m = fmt.Sprintf("condition is %s", machineCondition.Status)
					}

					machineMessages = append(machineMessages, fmt.Sprintf("  * %s: %s", machineCondition.Type, m))
				case metav1.ConditionUnknown:
					// Ignore unknown when the machine doesn't have a provider ID yet (which also implies infrastructure not ready).
					// Note: this avoids some noise when a new machine is provisioning; it is not possible to delay further
					// because the etcd member might join the cluster / control plane components might start even before
					// kubelet registers the node to the API server (e.g. in case kubelet has issues to register itself).
					if machine.Spec.ProviderID == "" {
						rcpMachinesWithInfo.Insert(machine.Name)

						break
					}

					rcpMachinesWithUnknown.Insert(machine.Name)

					m := machineCondition.Message

					if m == "" {
						m = fmt.Sprintf("condition is %s", machineCondition.Status)
					}

					machineMessages = append(machineMessages, fmt.Sprintf("  * %s: %s", machineCondition.Type, m))
				}
			}
		}

		if len(machineMessages) > 0 {
			if conditionCount > 1 && len(conditionMessages) == 1 {
				message := "  * Control plane components: " + conditionMessages.UnsortedList()[0]
				messageMap[message] = append(messageMap[message], machine.Name)

				continue
			}

			message := strings.Join(machineMessages, "\n")
			messageMap[message] = append(messageMap[message], machine.Name)
		}
	}

	// compute the order of messages according to the number of machines reporting the same message.
	// Note: The list of object names is used as a secondary criteria to sort messages with the same number of objects.
	messageIndex := make([]string, 0, len(messageMap))
	for m := range messageMap {
		messageIndex = append(messageIndex, m)
	}

	sort.SliceStable(messageIndex, func(i, j int) bool {
		iSlice, jSlice := messageMap[messageIndex[i]], messageMap[messageIndex[j]]

		return len(iSlice) > len(jSlice) || (len(iSlice) == len(jSlice) && strings.Join(iSlice, ",") < strings.Join(jSlice, ","))
	})

	// Build the message
	messages := []string{}

	for _, message := range messageIndex {
		machines := messageMap[message]
		machinesMessage := "Machine"

		if len(messageMap[message]) > 1 {
			machinesMessage += "s"
		}

		sort.Strings(machines)
		machinesMessage += " " + clog.ListToString(machines, func(s string) string { return s }, 3)

		messages = append(messages, fmt.Sprintf("* %s:\n%s", machinesMessage, message))
	}

	// Append messages impacting RCP as a whole, if any
	if len(input.rcpErrors) > 0 {
		messages = append(messages, input.rcpErrors...)
	}

	message := strings.Join(messages, "\n")

	// In case of at least one machine with errors or RCP level errors (nodes without machines), report false.
	if len(input.rcpErrors) > 0 || len(rcpMachinesWithErrors) > 0 {
		conditions.Set(input.controlPlane.RCP, metav1.Condition{
			Type:    input.condition,
			Status:  metav1.ConditionFalse,
			Reason:  input.falseReason,
			Message: message,
		})

		return
	}

	// Otherwise, if there is at least one machine with unknown, report unknown.
	if len(rcpMachinesWithUnknown) > 0 {
		conditions.Set(input.controlPlane.RCP, metav1.Condition{
			Type:    input.condition,
			Status:  metav1.ConditionUnknown,
			Reason:  input.unknownReason,
			Message: message,
		})

		return
	}

	// In case of no errors, no unknown, and at least one machine with info, report true.
	if len(rcpMachinesWithInfo) > 0 {
		conditions.Set(input.controlPlane.RCP, metav1.Condition{
			Type:   input.condition,
			Status: metav1.ConditionTrue,
			Reason: input.trueReason,
		})

		return
	}

	// This last case should happen only if there are no provisioned machines.
	conditions.Set(input.controlPlane.RCP, metav1.Condition{
		Type:    input.condition,
		Status:  metav1.ConditionUnknown,
		Reason:  input.unknownReason,
		Message: fmt.Sprintf("No Machines reporting %s status", input.note),
	})
}
