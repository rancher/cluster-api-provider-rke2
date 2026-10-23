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
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	kubeyaml "sigs.k8s.io/yaml"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/pkg/rke2"
)

func TestJoinWorkerNodeUninitializedTaint(t *testing.T) {
	tests := []struct {
		name       string
		nodeTaints []string
		want       []string
	}{
		{
			name:       "adds the taint after the configured taints",
			nodeTaints: []string{"example.com/dedicated=gpu:NoSchedule"},
			want:       []string{"example.com/dedicated=gpu:NoSchedule", "node.cluster.x-k8s.io/uninitialized:NoSchedule"},
		},
		{
			name:       "keeps a configured taint with the same key and effect",
			nodeTaints: []string{"node.cluster.x-k8s.io/uninitialized=true:NoSchedule"},
			want:       []string{"node.cluster.x-k8s.io/uninitialized=true:NoSchedule"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			tokenSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster-token", Namespace: "default"},
				Data:       map[string][]byte{"value": []byte("test-token")},
			}
			r := &RKE2ConfigReconciler{Client: fake.NewClientBuilder().WithObjects(tokenSecret).Build()}
			scope := &Scope{
				Logger: logr.Discard(),
				Config: &bootstrapv1.RKE2Config{
					ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "default"},
					Spec: bootstrapv1.RKE2ConfigSpec{
						AgentConfig: bootstrapv1.RKE2AgentConfig{NodeTaints: tt.nodeTaints},
					},
				},
				Machine: &clusterv1.Machine{Spec: clusterv1.MachineSpec{Version: "v1.34.2+rke2r1"}},
				Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"}},
				ControlPlane: &controlplanev1.RKE2ControlPlane{
					Status: controlplanev1.RKE2ControlPlaneStatus{AvailableServerIPs: []string{"10.0.0.1"}},
				},
			}

			_, err := r.joinWorker(t.Context(), scope)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(scope.Config.Spec.AgentConfig.NodeTaints).To(Equal(tt.nodeTaints))

			bootstrapSecret := &corev1.Secret{}
			g.Expect(r.Get(t.Context(), client.ObjectKeyFromObject(scope.Config), bootstrapSecret)).To(Succeed())

			var userData struct {
				WriteFiles []bootstrapv1.File `json:"write_files"`
			}
			g.Expect(kubeyaml.Unmarshal(bootstrapSecret.Data["value"], &userData)).To(Succeed())

			var config struct {
				NodeTaints []string `json:"node-taint"`
			}
			for _, file := range userData.WriteFiles {
				if file.Path == rke2.DefaultRKE2ConfigLocation {
					g.Expect(kubeyaml.Unmarshal([]byte(file.Content), &config)).To(Succeed())
				}
			}
			g.Expect(config.NodeTaints).To(Equal(tt.want))
		})
	}
}
