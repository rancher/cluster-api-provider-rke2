/*
Copyright 2024 SUSE LLC.

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

package v1beta1

import (
	"testing"

	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
)

func TestRKE2Config_ValidateCreate(t *testing.T) {
	tests := []struct {
		name      string
		spec      *RKE2ConfigSpec
		expectErr bool
	}{
		{
			name: "private registry no tls or auth secret",
			spec: &RKE2ConfigSpec{
				PrivateRegistriesConfig: Registry{
					Configs: map[string]RegistryConfig{
						"testing.io": {},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "private registry empty tls",
			spec: &RKE2ConfigSpec{
				PrivateRegistriesConfig: Registry{
					Configs: map[string]RegistryConfig{
						"testing.io": {
							TLS: TLSConfig{},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "private registry empty auth",
			spec: &RKE2ConfigSpec{
				PrivateRegistriesConfig: Registry{
					Configs: map[string]RegistryConfig{
						"testing.io": {
							AuthSecret: v1.ObjectReference{},
						},
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			config := &RKE2Config{
				Spec: *tt.spec.DeepCopy(),
			}

			_, err := config.ValidateCreate()

			if tt.expectErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}
