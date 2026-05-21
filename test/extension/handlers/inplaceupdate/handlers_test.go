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
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
)

func newHandlers(t *testing.T) *ExtensionHandlers {
	t.Helper()
	c := fake.NewClientBuilder().Build()
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

// First call must seed the tracker and report in-progress (RetryAfterSeconds > 0).
func TestDoUpdateMachine_FirstCallInProgress(t *testing.T) {
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

// Once enough fake time has elapsed since the first call, the handler must
// report completion (RetryAfterSeconds == 0) and clear the per-machine tracker.
func TestDoUpdateMachine_CompletesAfterFakeDuration(t *testing.T) {
	g := NewWithT(t)
	h := newHandlers(t)

	key := "default/m1"
	// Pretend we started long enough ago that the random jitter cannot keep us
	// in the in-progress branch. The handler completes between 30 and 40 seconds.
	h.state.Store(key, time.Now().Add(-60*time.Second))

	req := &runtimehooksv1.UpdateMachineRequest{
		Desired: runtimehooksv1.UpdateMachineRequestObjects{Machine: baseMachine()},
	}

	resp := &runtimehooksv1.UpdateMachineResponse{}
	h.DoUpdateMachine(context.Background(), req, resp)

	g.Expect(resp.Status).To(Equal(runtimehooksv1.ResponseStatusSuccess))
	g.Expect(resp.RetryAfterSeconds).To(BeEquivalentTo(0))

	_, stillTracked := h.state.Load(key)
	g.Expect(stillTracked).To(BeFalse(), "tracker entry must be cleared after completion")
}
