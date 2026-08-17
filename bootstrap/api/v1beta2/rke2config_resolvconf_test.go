/*
Copyright 2026 SUSE LLC.

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

package v1beta2

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("RKE2Config resolv.conf validation", func() {
	newConfig := func(name string, agentConfig RKE2AgentConfig) *RKE2Config {
		return &RKE2Config{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: metav1.NamespaceDefault,
			},
			Spec: RKE2ConfigSpec{
				AgentConfig: agentConfig,
			},
		}
	}

	It("should accept resolvConfPath on its own", func() {
		config := newConfig("resolv-conf-path-only", RKE2AgentConfig{
			ResolvConfPath: "/run/systemd/resolve/resolv.conf",
		})

		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		Expect(k8sClient.Delete(ctx, config)).To(Succeed())
	})

	It("should accept resolvConf on its own", func() {
		config := newConfig("resolv-conf-only", RKE2AgentConfig{
			ResolvConf: &corev1.ObjectReference{
				Name:      "test",
				Namespace: metav1.NamespaceDefault,
			},
		})

		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		Expect(k8sClient.Delete(ctx, config)).To(Succeed())
	})

	It("should reject resolvConf and resolvConfPath set together", func() {
		config := newConfig("resolv-conf-both", RKE2AgentConfig{
			ResolvConf: &corev1.ObjectReference{
				Name:      "test",
				Namespace: metav1.NamespaceDefault,
			},
			ResolvConfPath: "/run/systemd/resolve/resolv.conf",
		})

		err := k8sClient.Create(ctx, config)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("only one of resolvConf or resolvConfPath may be set"))
	})

	It("should reject a relative resolvConfPath", func() {
		config := newConfig("resolv-conf-relative", RKE2AgentConfig{
			ResolvConfPath: "run/systemd/resolve/resolv.conf",
		})

		Expect(k8sClient.Create(ctx, config)).ToNot(Succeed())
	})
})
