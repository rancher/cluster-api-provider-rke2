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
	"testing"

	. "github.com/onsi/gomega"
	planapi "github.com/rancher/rancher/pkg/plan"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func secretWithPlan(planBytes []byte, state planapi.PlanState) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "m1-machine-plan"},
		Data:       map[string][]byte{},
	}
	if planBytes != nil {
		s.Data[planDataKey] = planBytes
	}
	if state != "" {
		s.Data[planapi.PlanStateKey] = []byte(state)
	}
	return s
}

func TestEvaluatePlanOutcome(t *testing.T) {
	desired := []byte(`{"instructions":[{"name":"a"}]}`)
	other := []byte(`{"instructions":[{"name":"b"}]}`)

	tests := []struct {
		name   string
		secret *corev1.Secret
		want   planOutcome
	}{
		{"no plan yet", secretWithPlan(nil, ""), planOutcomeNotSubmitted},
		{"matches, no state (legacy agent)", secretWithPlan(desired, ""), planOutcomeWaiting},
		{"matches, pending", secretWithPlan(desired, planapi.PlanStatePending), planOutcomeWaiting},
		{"matches, in-progress", secretWithPlan(desired, planapi.PlanStateInProgress), planOutcomeWaiting},
		{"matches, paused", secretWithPlan(desired, planapi.PlanStatePaused), planOutcomeWaiting},
		{"matches, succeeded", secretWithPlan(desired, planapi.PlanStateSucceeded), planOutcomeSucceeded},
		{"matches, failed", secretWithPlan(desired, planapi.PlanStateFailed), planOutcomeFailed},
		{"matches, canceled", secretWithPlan(desired, planapi.PlanStateCanceled), planOutcomeFailed},
		{"differs, terminal (previous plan done)", secretWithPlan(other, planapi.PlanStateSucceeded), planOutcomeNotSubmitted},
		{"differs, in-progress (foreign plan in flight)", secretWithPlan(other, planapi.PlanStateInProgress), planOutcomeWaiting},
		{"differs, pending (foreign plan in flight)", secretWithPlan(other, planapi.PlanStatePending), planOutcomeWaiting},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(evaluatePlanOutcome(tt.secret, desired)).To(Equal(tt.want))
		})
	}
}

// A legacy agent never writes plan-state, so applied-checksum/failed-checksum are the only
// signals available; without this fallback the hook would poll forever once the plan is applied.
func TestEvaluatePlanOutcome_LegacyChecksumFallback(t *testing.T) {
	desired := []byte(`{"instructions":[{"name":"a"}]}`)
	desiredChecksum := planapi.Checksum(desired)

	tests := []struct {
		name   string
		secret *corev1.Secret
		want   planOutcome
	}{
		{
			name: "applied-checksum matches desired",
			secret: secretWithLegacyChecksum(desired, map[string]string{
				appliedChecksumKey: desiredChecksum,
			}),
			want: planOutcomeSucceeded,
		},
		{
			name: "failed-checksum matches desired",
			secret: secretWithLegacyChecksum(desired, map[string]string{
				failedChecksumKey: desiredChecksum,
			}),
			want: planOutcomeFailed,
		},
		{
			name: "neither checksum recorded yet",
			secret: secretWithLegacyChecksum(desired, map[string]string{
				appliedChecksumKey: "some-other-checksum",
			}),
			want: planOutcomeWaiting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(evaluatePlanOutcome(tt.secret, desired)).To(Equal(tt.want))
		})
	}
}

func secretWithLegacyChecksum(planBytes []byte, data map[string]string) *corev1.Secret {
	s := secretWithPlan(planBytes, "")
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	return s
}

func TestWritePlan(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "m1-machine-plan"},
		Data: map[string][]byte{
			planapi.PlanStateKey: []byte(planapi.PlanStateSucceeded),
			"probe-statuses":     []byte(`{"foo":"bar"}`),
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing.DeepCopy()).Build()

	planBytes := []byte(`{"instructions":[{"name":"a"}]}`)
	g.Expect(writePlan(context.Background(), c, existing, planBytes)).To(Succeed())

	got := &corev1.Secret{}
	g.Expect(c.Get(context.Background(), client.ObjectKeyFromObject(existing), got)).To(Succeed())

	g.Expect(got.Data[planDataKey]).To(Equal(planBytes))
	g.Expect(got.Data[planapi.PlanStateKey]).To(Equal([]byte(planapi.PlanStatePending)))
	// Fields owned by system-agent must survive the merge patch untouched.
	g.Expect(got.Data["probe-statuses"]).To(Equal([]byte(`{"foo":"bar"}`)))
}
