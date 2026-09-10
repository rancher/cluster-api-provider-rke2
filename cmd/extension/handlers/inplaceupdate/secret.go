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

package inplaceupdate

import (
	"context"

	planapi "github.com/rancher/rancher/pkg/plan"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// planDataKey is the Secret data key holding the plan payload.
const planDataKey = "plan"

// Secret data keys written by system-agent on plan completion, regardless of plan-state support.
// Mirrors system-agent's k8splan.AppliedChecksumKey / FailedChecksumKey; not exported from the
// shared pkg/plan library since they're considered a transitional, agent-internal concept.
const (
	appliedChecksumKey = "applied-checksum"
	failedChecksumKey  = "failed-checksum"
)

// planOutcome summarizes a machine-plan Secret's plan-state relative to a desired plan.
type planOutcome int

const (
	// planOutcomeNotSubmitted means the Secret does not yet carry the desired plan content and no
	// other plan is currently in flight, so it is safe to submit it.
	planOutcomeNotSubmitted planOutcome = iota
	// planOutcomeWaiting means either the desired plan was submitted and has not reached a
	// terminal state yet, or a different plan is currently in flight and must not be clobbered.
	planOutcomeWaiting
	// planOutcomeSucceeded means system-agent applied the desired plan successfully.
	planOutcomeSucceeded
	// planOutcomeFailed means system-agent failed or canceled the desired plan.
	planOutcomeFailed
)

// evaluatePlanOutcome compares the Secret's current plan payload against desiredPlanBytes to
// decide whether the desired plan still needs to be (re)submitted, or whether system-agent has
// already picked it up and reached a particular state.
//
// Completion is read from applied-checksum/failed-checksum rather than plan-state: system-agent
// writes those two keys on every completion regardless of plan-state support (see
// buildSecretDataUpdates in system-agent's k8splan package), whereas plan-state itself can get
// stuck: writePlan unconditionally sets plan-state=pending on submission, and an agent that
// doesn't understand plan-state will never move it off "pending" — so gating completion on
// plan-state alone would poll forever against such an agent.
func evaluatePlanOutcome(secret *corev1.Secret, desiredPlanBytes []byte) planOutcome {
	currentState := planapi.PlanState(string(secret.Data[planapi.PlanStateKey]))
	desiredChecksum := planapi.Checksum(desiredPlanBytes)
	checksumMatches := planapi.Checksum(secret.Data[planDataKey]) == desiredChecksum

	if checksumMatches {
		switch {
		case string(secret.Data[appliedChecksumKey]) == desiredChecksum:
			return planOutcomeSucceeded
		case string(secret.Data[failedChecksumKey]) == desiredChecksum:
			return planOutcomeFailed
		default:
			return planOutcomeWaiting
		}
	}

	// Checksum differs: our desired plan hasn't been submitted. Per the machine-plan Secret
	// orchestration protocol, new content may only be written once the previous plan (if any)
	// has reached a terminal state; otherwise we'd clobber an in-flight execution.
	if currentState != "" && !currentState.IsTerminal() {
		return planOutcomeWaiting
	}

	return planOutcomeNotSubmitted
}

// writePlan merge-patches the desired plan content and a fresh plan-state=pending into the
// machine-plan Secret. A merge patch (RFC 7386) is used instead of a full Update so that only the
// "plan" and "plan-state" keys are touched: system-agent concurrently writes other keys
// to the same Secret, and a full Update would race with it, risking a resourceVersion conflict.
func writePlan(ctx context.Context, c client.Client, secret *corev1.Secret, planBytes []byte) error {
	base := secret.DeepCopy()

	updated := secret.DeepCopy()
	if updated.Data == nil {
		updated.Data = map[string][]byte{}
	}

	updated.Data[planDataKey] = planBytes
	updated.Data[planapi.PlanStateKey] = []byte(planapi.PlanStatePending)

	return c.Patch(ctx, updated, client.MergeFrom(base))
}
