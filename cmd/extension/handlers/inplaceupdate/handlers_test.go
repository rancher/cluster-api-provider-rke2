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
	"encoding/base64"
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	planapi "github.com/rancher/rancher/pkg/plan"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"

	planv1alpha1 "github.com/rancher/rancher/pkg/plan/api/plan.cattle.io/v1alpha1"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
)

func newHandlers(t *testing.T, objects ...client.Object) *ExtensionHandlers {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = bootstrapv1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	return NewExtensionHandlers(c)
}

func mustRaw(t *testing.T, obj runtime.Object) runtime.RawExtension {
	t.Helper()
	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("failed to marshal %T: %v", obj, err)
	}
	return runtime.RawExtension{Raw: raw}
}

func baseMachine() clusterv1.Machine {
	return clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "m1"},
		Spec: clusterv1.MachineSpec{
			ClusterName: "test",
			Version:     "v1.34.2",
		},
	}
}

func baseRKE2Config(files ...bootstrapv1.File) *bootstrapv1.RKE2Config {
	return &bootstrapv1.RKE2Config{
		TypeMeta: metav1.TypeMeta{
			APIVersion: bootstrapv1.GroupVersion.String(),
			Kind:       "RKE2Config",
		},
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "m1"},
		Spec: bootstrapv1.RKE2ConfigSpec{
			Files: files,
		},
	}
}

func canUpdateRequest(current, desired runtimehooksv1.CanUpdateMachineRequestObjects) *runtimehooksv1.CanUpdateMachineRequest {
	return &runtimehooksv1.CanUpdateMachineRequest{Current: current, Desired: desired}
}

func TestDoCanUpdateMachine_AbsorbsFilesAndVersion(t *testing.T) {
	g := NewWithT(t)
	h := newHandlers(t)

	currentMachine := baseMachine()
	desiredMachine := baseMachine()
	desiredMachine.Spec.Version = "v1.34.3"

	currentRKE2 := baseRKE2Config()
	desiredRKE2 := baseRKE2Config(bootstrapv1.File{Path: "/etc/rke2-test", Content: "after"})

	req := canUpdateRequest(
		runtimehooksv1.CanUpdateMachineRequestObjects{
			Machine:         currentMachine,
			BootstrapConfig: mustRaw(t, currentRKE2),
		},
		runtimehooksv1.CanUpdateMachineRequestObjects{
			Machine:         desiredMachine,
			BootstrapConfig: mustRaw(t, desiredRKE2),
		},
	)

	resp := &runtimehooksv1.CanUpdateMachineResponse{}
	h.DoCanUpdateMachine(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusSuccess))
	g.Expect(resp.Message).To(BeEmpty())

	// We don't pin the exact JSON Patch shape (jsonpatch v2 may emit either an
	// `add /spec/<field>` op or an `add /spec` with the full object). We only
	// require the patch to be JSONPatchType and to carry the new values.
	g.Expect(resp.MachinePatch.PatchType).To(Equal(runtimehooksv1.JSONPatchType))
	g.Expect(string(resp.MachinePatch.Patch)).To(ContainSubstring("v1.34.3"))

	g.Expect(resp.BootstrapConfigPatch.PatchType).To(Equal(runtimehooksv1.JSONPatchType))
	g.Expect(string(resp.BootstrapConfigPatch.Patch)).To(ContainSubstring("/etc/rke2-test"))
	g.Expect(string(resp.BootstrapConfigPatch.Patch)).To(ContainSubstring(`"content":"after"`))

	// The extension is infra-agnostic and never claims an infra field, so the
	// InfrastructureMachinePatch must stay undefined in the response.
	g.Expect(resp.InfrastructureMachinePatch.IsDefined()).To(BeFalse())
}

// Spec changes outside the allowlist (e.g. RKE2ConfigSpec.PreRKE2Commands)
// must NOT show up in any of the patches. The CP controller takes that as a
// signal that the extension can't fully absorb the diff and falls back to a
// rolling rollout.
func TestDoCanUpdateMachine_DoesNotAbsorbDisallowedFields(t *testing.T) {
	g := NewWithT(t)
	h := newHandlers(t)

	currentRKE2 := baseRKE2Config()
	desiredRKE2 := baseRKE2Config()
	desiredRKE2.Spec.PreRKE2Commands = []string{"echo hi"}

	req := canUpdateRequest(
		runtimehooksv1.CanUpdateMachineRequestObjects{
			Machine:         baseMachine(),
			BootstrapConfig: mustRaw(t, currentRKE2),
		},
		runtimehooksv1.CanUpdateMachineRequestObjects{
			Machine:         baseMachine(),
			BootstrapConfig: mustRaw(t, desiredRKE2),
		},
	)

	resp := &runtimehooksv1.CanUpdateMachineResponse{}
	h.DoCanUpdateMachine(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusSuccess))
	g.Expect(string(resp.BootstrapConfigPatch.Patch)).ToNot(ContainSubstring("preRKE2Commands"),
		"PreRKE2Commands is outside the allowlist and must not appear in the patch")
}

func baseMachineSet() clusterv1.MachineSet {
	return clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "ms1"},
		Spec: clusterv1.MachineSetSpec{
			ClusterName: "test",
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					ClusterName: "test",
					Version:     "v1.34.2",
				},
			},
		},
	}
}

func baseRKE2ConfigTemplate(files ...bootstrapv1.File) *bootstrapv1.RKE2ConfigTemplate {
	return &bootstrapv1.RKE2ConfigTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: bootstrapv1.GroupVersion.String(),
			Kind:       "RKE2ConfigTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "ms1"},
		Spec: bootstrapv1.RKE2ConfigTemplateSpec{
			Template: bootstrapv1.RKE2ConfigTemplateResource{
				Spec: bootstrapv1.RKE2ConfigSpec{
					Files: files,
				},
			},
		},
	}
}

func TestDoCanUpdateMachineSet_AbsorbsFilesAndVersionAtTemplateLevel(t *testing.T) {
	g := NewWithT(t)
	h := newHandlers(t)

	currentMS := baseMachineSet()
	desiredMS := baseMachineSet()
	desiredMS.Spec.Template.Spec.Version = "v1.34.3"

	currentRKE2Template := baseRKE2ConfigTemplate()
	desiredRKE2Template := baseRKE2ConfigTemplate(bootstrapv1.File{Path: "/etc/rke2-test", Content: "after"})

	req := &runtimehooksv1.CanUpdateMachineSetRequest{
		Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
			MachineSet:              currentMS,
			BootstrapConfigTemplate: mustRaw(t, currentRKE2Template),
		},
		Desired: runtimehooksv1.CanUpdateMachineSetRequestObjects{
			MachineSet:              desiredMS,
			BootstrapConfigTemplate: mustRaw(t, desiredRKE2Template),
		},
	}

	resp := &runtimehooksv1.CanUpdateMachineSetResponse{}
	h.DoCanUpdateMachineSet(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusSuccess))
	g.Expect(resp.Message).To(BeEmpty())

	g.Expect(resp.MachineSetPatch.PatchType).To(Equal(runtimehooksv1.JSONPatchType))
	g.Expect(string(resp.MachineSetPatch.Patch)).To(ContainSubstring("v1.34.3"))

	g.Expect(resp.BootstrapConfigTemplatePatch.PatchType).To(Equal(runtimehooksv1.JSONPatchType))
	g.Expect(string(resp.BootstrapConfigTemplatePatch.Patch)).To(ContainSubstring("/etc/rke2-test"))
	g.Expect(string(resp.BootstrapConfigTemplatePatch.Patch)).To(ContainSubstring(`"content":"after"`))

	// The extension is infra-agnostic and never claims an infra field, so the
	// InfrastructureMachineTemplatePatch must stay undefined in the response.
	g.Expect(resp.InfrastructureMachineTemplatePatch.IsDefined()).To(BeFalse())
}

// Disallowed template-level fields (e.g. RKE2ConfigSpec.PreRKE2Commands) must
// NOT appear in the BootstrapConfigTemplate patch, so the MD controller falls
// back to a rolling rollout for changes outside our allowlist.
func TestDoCanUpdateMachineSet_DoesNotAbsorbDisallowedFields(t *testing.T) {
	g := NewWithT(t)
	h := newHandlers(t)

	currentRKE2Template := baseRKE2ConfigTemplate()
	desiredRKE2Template := baseRKE2ConfigTemplate()
	desiredRKE2Template.Spec.Template.Spec.PreRKE2Commands = []string{"echo hi"}

	req := &runtimehooksv1.CanUpdateMachineSetRequest{
		Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
			MachineSet:              baseMachineSet(),
			BootstrapConfigTemplate: mustRaw(t, currentRKE2Template),
		},
		Desired: runtimehooksv1.CanUpdateMachineSetRequestObjects{
			MachineSet:              baseMachineSet(),
			BootstrapConfigTemplate: mustRaw(t, desiredRKE2Template),
		},
	}

	resp := &runtimehooksv1.CanUpdateMachineSetResponse{}
	h.DoCanUpdateMachineSet(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusSuccess))
	g.Expect(string(resp.BootstrapConfigTemplatePatch.Patch)).ToNot(ContainSubstring("preRKE2Commands"),
		"PreRKE2Commands is outside the allowlist and must not appear in the template patch")
}

// When no machine-plan Secret exists the system-agent has not yet registered; the handler must
// ask CAPI to retry until registration completes.
func TestDoUpdateMachine_SecretNotFound_Retries(t *testing.T) {
	g := NewWithT(t)
	h := newHandlers(t)

	req := &runtimehooksv1.UpdateMachineRequest{
		Desired: runtimehooksv1.UpdateMachineRequestObjects{Machine: baseMachine()},
	}

	resp := &runtimehooksv1.UpdateMachineResponse{}
	h.DoUpdateMachine(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusSuccess))
	g.Expect(resp.RetryAfterSeconds).To(BeNumerically(">", 0))
}

// Once the machine-plan Secret exists the handler must report that the update is in progress
// and continue retrying until the plan is executed.
func TestDoUpdateMachine_SecretFound_ReturnsInProgress(t *testing.T) {
	g := NewWithT(t)
	m := baseMachine()
	h := newHandlers(t, machinePlanSecret(m.Namespace, m.Name))

	req := &runtimehooksv1.UpdateMachineRequest{
		Desired: runtimehooksv1.UpdateMachineRequestObjects{Machine: m},
	}

	resp := &runtimehooksv1.UpdateMachineResponse{}
	h.DoUpdateMachine(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusSuccess))
	g.Expect(resp.RetryAfterSeconds).To(BeNumerically(">", 0))
}

// A Secret with matching labels but a different type must not satisfy the lookup — only Secrets
// of type rke.cattle.io/machine-plan are valid plan carriers.
func TestDoUpdateMachine_SecretWrongType_Retries(t *testing.T) {
	g := NewWithT(t)
	m := baseMachine()

	wrongType := machinePlanSecret(m.Namespace, m.Name)
	wrongType.Type = corev1.SecretTypeOpaque
	h := newHandlers(t, wrongType)

	req := &runtimehooksv1.UpdateMachineRequest{
		Desired: runtimehooksv1.UpdateMachineRequestObjects{Machine: m},
	}

	resp := &runtimehooksv1.UpdateMachineResponse{}
	h.DoUpdateMachine(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusSuccess))
	g.Expect(resp.RetryAfterSeconds).To(BeNumerically(">", 0))
}

func machinePlanSecret(namespace, machineName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      machineName + "-machine-plan",
			Labels: map[string]string{
				planv1alpha1.MachineLifecycleGroupLabel: clusterv1.GroupVersion.Group,
				planv1alpha1.MachineLifecycleKindLabel:  "Machine",
				planv1alpha1.MachineLifecycleNameLabel:  machineName,
			},
		},
		Type: machinePlanSecretType,
	}
}

// When the machine-plan Secret exists but has no plan submitted yet, the handler must build and
// write the upgrade plan (plan-state=pending) and ask CAPI to retry.
func TestDoUpdateMachine_NoPlanYet_SubmitsPlanAndRetries(t *testing.T) {
	g := NewWithT(t)
	m := baseMachine()
	secret := machinePlanSecret(m.Namespace, m.Name)
	h := newHandlers(t, secret)

	req := &runtimehooksv1.UpdateMachineRequest{
		Desired: runtimehooksv1.UpdateMachineRequestObjects{Machine: m},
	}

	resp := &runtimehooksv1.UpdateMachineResponse{}
	h.DoUpdateMachine(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusSuccess))
	g.Expect(resp.RetryAfterSeconds).To(BeNumerically(">", 0))

	got := &corev1.Secret{}
	g.Expect(h.client.Get(context.Background(), client.ObjectKeyFromObject(secret), got)).To(Succeed())
	g.Expect(got.Data[planDataKey]).NotTo(BeEmpty())
	g.Expect(string(got.Data[planapi.PlanStateKey])).To(Equal(string(planapi.PlanStatePending)))
}

// While the submitted plan has not reached a terminal state, the handler must keep retrying
// without resubmitting (the plan content must stay untouched).
func TestDoUpdateMachine_PlanInProgress_Retries(t *testing.T) {
	g := NewWithT(t)
	m := baseMachine()

	desiredPlan, err := buildUpgradePlan(&m, nil)
	g.Expect(err).NotTo(HaveOccurred())
	planBytes, err := json.Marshal(desiredPlan)
	g.Expect(err).NotTo(HaveOccurred())

	secret := machinePlanSecret(m.Namespace, m.Name)
	secret.Data = map[string][]byte{
		planDataKey:          planBytes,
		planapi.PlanStateKey: []byte(planapi.PlanStateInProgress),
		"probe-statuses":     []byte(`{"foo":"bar"}`),
	}
	h := newHandlers(t, secret)

	req := &runtimehooksv1.UpdateMachineRequest{
		Desired: runtimehooksv1.UpdateMachineRequestObjects{Machine: m},
	}

	resp := &runtimehooksv1.UpdateMachineResponse{}
	h.DoUpdateMachine(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusSuccess))
	g.Expect(resp.RetryAfterSeconds).To(BeNumerically(">", 0))

	got := &corev1.Secret{}
	g.Expect(h.client.Get(context.Background(), client.ObjectKeyFromObject(secret), got)).To(Succeed())
	g.Expect(got.Data["probe-statuses"]).To(Equal([]byte(`{"foo":"bar"}`)), "unrelated agent-owned keys must not change")
}

// Once system-agent marks the submitted plan as succeeded, the hook must report a terminal
// success (no further retries), so CAPI marks the Machine as UpToDate.
func TestDoUpdateMachine_PlanSucceeded_ReportsUpToDate(t *testing.T) {
	g := NewWithT(t)
	m := baseMachine()

	desiredPlan, err := buildUpgradePlan(&m, nil)
	g.Expect(err).NotTo(HaveOccurred())
	planBytes, err := json.Marshal(desiredPlan)
	g.Expect(err).NotTo(HaveOccurred())

	secret := machinePlanSecret(m.Namespace, m.Name)
	secret.Data = map[string][]byte{
		planDataKey:          planBytes,
		planapi.PlanStateKey: []byte(planapi.PlanStateSucceeded),
	}
	h := newHandlers(t, secret)

	req := &runtimehooksv1.UpdateMachineRequest{
		Desired: runtimehooksv1.UpdateMachineRequestObjects{Machine: m},
	}

	resp := &runtimehooksv1.UpdateMachineResponse{}
	h.DoUpdateMachine(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusSuccess))
	g.Expect(resp.RetryAfterSeconds).To(BeZero(), "a terminal success must not ask CAPI to retry")
}

// When system-agent marks the submitted plan as failed, the hook must report Failure so the
// caller can decide on remediation.
func TestDoUpdateMachine_PlanFailed_ReportsFailure(t *testing.T) {
	g := NewWithT(t)
	m := baseMachine()

	desiredPlan, err := buildUpgradePlan(&m, nil)
	g.Expect(err).NotTo(HaveOccurred())
	planBytes, err := json.Marshal(desiredPlan)
	g.Expect(err).NotTo(HaveOccurred())

	secret := machinePlanSecret(m.Namespace, m.Name)
	secret.Data = map[string][]byte{
		planDataKey:          planBytes,
		planapi.PlanStateKey: []byte(planapi.PlanStateFailed),
	}
	h := newHandlers(t, secret)

	req := &runtimehooksv1.UpdateMachineRequest{
		Desired: runtimehooksv1.UpdateMachineRequestObjects{Machine: m},
	}

	resp := &runtimehooksv1.UpdateMachineResponse{}
	h.DoUpdateMachine(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusFailure))
}

// A machine with no desired version must fail fast rather than silently doing nothing.
func TestDoUpdateMachine_NoDesiredVersion_ReportsFailure(t *testing.T) {
	g := NewWithT(t)
	m := baseMachine()
	m.Spec.Version = ""
	secret := machinePlanSecret(m.Namespace, m.Name)
	h := newHandlers(t, secret)

	req := &runtimehooksv1.UpdateMachineRequest{
		Desired: runtimehooksv1.UpdateMachineRequestObjects{Machine: m},
	}

	resp := &runtimehooksv1.UpdateMachineResponse{}
	h.DoUpdateMachine(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusFailure))
}

// DoCanUpdateMachine claims RKE2ConfigSpec.Files as an in-place-updatable field, so DoUpdateMachine
// must actually deliver desired file content through the submitted plan, not just the version bump.
func TestDoUpdateMachine_SubmitsDesiredBootstrapFiles(t *testing.T) {
	g := NewWithT(t)
	m := baseMachine()
	secret := machinePlanSecret(m.Namespace, m.Name)
	h := newHandlers(t, secret)

	desiredRKE2 := baseRKE2Config(bootstrapv1.File{Path: "/etc/rke2-test", Content: "after", Permissions: "0644"})

	req := &runtimehooksv1.UpdateMachineRequest{
		Desired: runtimehooksv1.UpdateMachineRequestObjects{
			Machine:         m,
			BootstrapConfig: mustRaw(t, desiredRKE2),
		},
	}

	resp := &runtimehooksv1.UpdateMachineResponse{}
	h.DoUpdateMachine(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusSuccess))

	got := &corev1.Secret{}
	g.Expect(h.client.Get(context.Background(), client.ObjectKeyFromObject(secret), got)).To(Succeed())

	submittedPlan, err := planapi.Parse(got.Data[planDataKey])
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(submittedPlan.Files).To(HaveLen(1))
	g.Expect(submittedPlan.Files[0].Path).To(Equal("/etc/rke2-test"))
	g.Expect(submittedPlan.Files[0].Content).To(Equal(base64.StdEncoding.EncodeToString([]byte("after"))))
}
