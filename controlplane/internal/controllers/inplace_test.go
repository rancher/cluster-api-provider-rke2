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

package controllers

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/record"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimecatalog "sigs.k8s.io/cluster-api/api/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/test/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/rke2"
)

// fakeRuntimeClient pretends to be a single registered CanUpdateMachine extension.
// It answers every call with err, or with an empty successful response (meaning the
// extension cannot update anything in-place).
type fakeRuntimeClient struct {
	runtimeclient.Client
	err    error
	called bool
}

func (f *fakeRuntimeClient) GetAllExtensions(context.Context, runtimecatalog.Hook, client.Object) ([]string, error) {
	return []string{"can-update-machine"}, nil
}

func (f *fakeRuntimeClient) CallExtension(_ context.Context, _ runtimecatalog.Hook, _ client.Object, _ string,
	_ runtimehooksv1.RequestObject, resp runtimehooksv1.ResponseObject, _ ...runtimeclient.CallExtensionOption,
) error {
	f.called = true
	resp.SetStatus(runtimehooksv1.ResponseStatusSuccess)

	return f.err
}

var _ = Describe("In-place updates", func() {
	var (
		ns            *corev1.Namespace
		machine       *clusterv1.Machine
		upToDate      rke2.UpToDateResult
		runtimeClient *fakeRuntimeClient
		r             *RKE2ControlPlaneReconciler
	)

	BeforeEach(func() {
		featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), feature.Gates, feature.InPlaceUpdates, true)

		var err error
		ns, err = testEnv.CreateNamespace(ctx, "test-inplace")
		Expect(err).ToNot(HaveOccurred())

		rke2Config := &bootstrapv1.RKE2Config{ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: ns.Name}}
		Expect(testEnv.Create(ctx, rke2Config)).To(Succeed())

		infraMachine := &unstructured.Unstructured{}
		infraMachine.SetGroupVersionKind(builder.InfrastructureGroupVersion.WithKind(builder.GenericInfrastructureMachineKind))
		infraMachine.SetName("m1")
		infraMachine.SetNamespace(ns.Name)
		Expect(testEnv.Create(ctx, infraMachine)).To(Succeed())

		machine = &clusterv1.Machine{
			Name: "m1", Namespace: ns.Name,
			Spec: clusterv1.MachineSpec{
				ClusterName: "test",
				Version:     "v1.34.2+rke2r1",
				Bootstrap:   clusterv1.Bootstrap{DataSecretName: ptr.To("m1")},
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: builder.InfrastructureGroupVersion.Group,
					Kind:     builder.GenericInfrastructureMachineKind,
					Name:     "m1",
				},
			},
			Status: clusterv1.MachineStatus{
				Conditions: []metav1.Condition{
					{Type: controlplanev1.RKE2ControlPlaneMachineAgentHealthyCondition, Status: metav1.ConditionTrue},
					{Type: controlplanev1.RKE2ControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue},
				},
			},
		}

		// The user bumped the version, which is the only difference between current and desired state.
		desiredMachine := machine.DeepCopy()
		desiredMachine.Spec.Version = "v1.35.0+rke2r1"

		upToDate = rke2.UpToDateResult{
			EligibleForInPlaceUpdate: true,
			DesiredMachine:           desiredMachine,
			CurrentInfraMachine:      infraMachine,
			DesiredInfraMachine:      infraMachine.DeepCopy(),
			CurrentRKE2Config:        rke2Config,
			DesiredRKE2Config:        rke2Config.DeepCopy(),
		}

		runtimeClient = &fakeRuntimeClient{}
		r = &RKE2ControlPlaneReconciler{
			Client:        testEnv.GetClient(),
			RuntimeClient: runtimeClient,
			recorder:      record.NewFakeRecorder(10),
		}
	})

	AfterEach(func() {
		Expect(testEnv.Cleanup(ctx, ns)).To(Succeed())
	})

	controlPlane := func() *rke2.ControlPlane {
		return &rke2.ControlPlane{
			RCP:      &controlplanev1.RKE2ControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "rcp", Namespace: ns.Name}},
			Machines: collections.FromMachines(machine),
		}
	}

	It("Should not roll out the Machine if the extension fails", func() {
		runtimeClient.err = errors.New("extension unavailable")

		fallbackToScaleDown, _, err := r.tryInPlaceUpdate(ctx, controlPlane(), machine, upToDate)
		Expect(err).To(MatchError(ContainSubstring("extension unavailable")))
		Expect(fallbackToScaleDown).To(BeFalse(), "a failing extension must never lead to a rollout")
	})

	It("Should roll out the Machine if the extension cannot update it in-place", func() {
		fallbackToScaleDown, _, err := r.tryInPlaceUpdate(ctx, controlPlane(), machine, upToDate)
		Expect(err).ToNot(HaveOccurred())
		Expect(fallbackToScaleDown).To(BeTrue())
	})

	It("Should roll out the Machine without asking the extension if the Machine is unhealthy", func() {
		machine.Status.Conditions[0].Status = metav1.ConditionFalse

		fallbackToScaleDown, _, err := r.tryInPlaceUpdate(ctx, controlPlane(), machine, upToDate)
		Expect(err).ToNot(HaveOccurred())
		Expect(fallbackToScaleDown).To(BeTrue())
		Expect(runtimeClient.called).To(BeFalse(), "no in-place update must be tried on an unhealthy Machine")
	})
})
