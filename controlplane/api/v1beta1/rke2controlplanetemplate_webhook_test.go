/*
Copyright 2024 SUSE.
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
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

func TestRKE2ControlPlaneTemplateValidateCreate(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name          string
		inputTemplate *RKE2ControlPlaneTemplate
		wantErr       bool
	}{
		{
			name: "don't allow RKE2ControlPlaneTemplate with invalid CNI",
			inputTemplate: &RKE2ControlPlaneTemplate{
				Spec: RKE2ControlPlaneTemplateSpec{
					Template: RKE2ControlPlaneTemplateResource{
						Spec: RKE2ControlPlaneSpec{
							ServerConfig: RKE2ServerConfig{
								CNIMultusEnable: true,
								CNI:             "",
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	validator := RKE2ControlPlaneTemplateCustomValidator{}
	for _, test := range tests {
		tt := test
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			warn, err := validator.ValidateCreate(context.Background(), tt.inputTemplate)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(warn).To(BeNil())
		})
	}
}

func TestRKE2ControlPlaneTemplateValidateUpdate(t *testing.T) {
	g := NewWithT(t)

	replicas := int32(3)
	tests := []struct {
		name        string
		newTemplate *RKE2ControlPlaneTemplate
		oldTemplate *RKE2ControlPlaneTemplate
		wantErr     bool
	}{
		{
			name: "RKE2ControlPlaneTemplate with immutable spec",
			newTemplate: &RKE2ControlPlaneTemplate{
				Spec: RKE2ControlPlaneTemplateSpec{
					Template: RKE2ControlPlaneTemplateResource{
						Spec: RKE2ControlPlaneSpec{
							Replicas: &replicas,
						},
					},
				},
			},
			oldTemplate: &RKE2ControlPlaneTemplate{
				Spec: RKE2ControlPlaneTemplateSpec{
					Template: RKE2ControlPlaneTemplateResource{
						Spec: RKE2ControlPlaneSpec{
							Replicas: &replicas,
						},
					},
				},
			},
			wantErr: false,
		},
	}
	validator := RKE2ControlPlaneTemplateCustomValidator{}
	for _, test := range tests {
		tt := test
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			warn, err := validator.ValidateUpdate(context.Background(), tt.oldTemplate, tt.newTemplate)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(warn).To(BeNil())
		})
	}
}
