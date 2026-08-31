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
	"testing"

	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
)

// TestMatchesMachineSpecMissingInfraMachineTemplate verifies that a missing InfraMachineTemplate does not block
// the deletion of an RKE2ControlPlane, which can happen when all the cluster resources are deleted at once
// (e.g. `kubectl delete -f cluster.yaml`) and the template is removed before the control plane is gone.
func TestMatchesMachineSpecMissingInfraMachineTemplate(t *testing.T) {
	infraTemplateRef := clusterv1.ContractVersionedObjectReference{
		APIGroup: "infrastructure.cluster.x-k8s.io",
		Kind:     "Metal3MachineTemplate",
		Name:     "sample-cluster-controlplane",
	}

	newRCP := func(deleting bool) *controlplanev1.RKE2ControlPlane {
		rcp := &controlplanev1.RKE2ControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "sample-cluster",
			},
			Spec: controlplanev1.RKE2ControlPlaneSpec{
				Version: "v1.34.2+rke2r1",
				MachineTemplate: controlplanev1.RKE2ControlPlaneMachineTemplate{
					Spec: controlplanev1.RKE2ControlPlaneMachineTemplateSpec{
						InfrastructureRef: infraTemplateRef,
					},
				},
			},
		}

		if deleting {
			rcp.DeletionTimestamp = ptr.To(metav1.Now())
			rcp.Finalizers = []string{controlplanev1.RKE2ControlPlaneFinalizer}
		}

		return rcp
	}

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "sample-cluster"},
	}

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "sample-cluster-k6qrm"},
		Spec: clusterv1.MachineSpec{
			ClusterName: cluster.Name,
			Version:     "v1.34.2+rke2r1",
		},
	}

	// The InfraMachine still exists, the Metal3MachineTemplate it was cloned from does not.
	infraMachine := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta2",
		"kind":       "Metal3Machine",
		"metadata": map[string]interface{}{
			"name":      machine.Name,
			"namespace": machine.Namespace,
		},
	}}
	infraConfigs := map[string]*unstructured.Unstructured{machine.Name: infraMachine}

	// The CRD is still around, so resolving the contract apiVersion succeeds and the template itself is fetched.
	infraTemplateCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "metal3machinetemplates.infrastructure.cluster.x-k8s.io",
			Labels: map[string]string{"cluster.x-k8s.io/v1beta2": "v1beta2"},
		},
	}

	scheme := runtime.NewScheme()
	g := NewWithT(t)
	g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(infraTemplateCRD).Build()

	t.Run("is tolerated when the RKE2ControlPlane is deleting", func(t *testing.T) {
		g := NewWithT(t)
		res := &UpToDateResult{CurrentInfraMachine: infraMachine}

		matches, logMessages, conditionMessages, err := matchesMachineSpec(
			t.Context(), c, cluster, infraConfigs, nil, newRCP(true), machine, res)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(matches).To(BeTrue())
		g.Expect(logMessages).To(BeEmpty())
		g.Expect(conditionMessages).To(BeEmpty())
	})

	t.Run("is an error when the RKE2ControlPlane is not deleting", func(t *testing.T) {
		g := NewWithT(t)
		res := &UpToDateResult{CurrentInfraMachine: infraMachine}

		_, _, _, err := matchesMachineSpec(
			t.Context(), c, cluster, infraConfigs, nil, newRCP(false), machine, res)

		g.Expect(err).To(HaveOccurred())
	})
}
