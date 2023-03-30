/*
Copyright 2023 SUSE.

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

package ignition

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1alpha1"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/internal/cloudinit"
)

func TestIgnition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ignition Suite")
}

var additionalIgnition = `---
systemd:
  units:
    - name: test.service
      enabled: true
      contents: |
        [Unit]
        Description=test
        [Service]
        ExecStart=/etc/test.sh
        [Install]
        WantedBy=test.target
`

var _ = Describe("NewJoinWorker", func() {
	var input *JoinWorkerInput

	BeforeEach(func() {
		input = &JoinWorkerInput{
			BaseUserData: &cloudinit.BaseUserData{
				RKE2Version: "v1.21.3+rke2r1",
				WriteFiles: []bootstrapv1.File{
					{
						Path:        "/test/file",
						Content:     "test",
						Permissions: "0644",
					},
				},
				ConfigFile: bootstrapv1.File{
					Path:        "/test/config",
					Content:     "test",
					Permissions: "0644",
				},
			},
			AdditionalIgnition: &bootstrapv1.AdditionalUserData{
				Config: additionalIgnition,
				Strict: true,
			},
		}
	})

	It("should return ignition data for worker", func() {
		ignition, err := NewJoinWorker(input)
		Expect(err).ToNot(HaveOccurred())
		Expect(ignition).ToNot(BeNil())
	})

	It("should return error if input is nil", func() {
		input = nil
		ignition, err := NewJoinWorker(input)
		Expect(err).To(HaveOccurred())
		Expect(ignition).To(BeNil())
	})

	It("should return error if base userdata is nil", func() {
		input.BaseUserData = nil
		ignition, err := NewJoinWorker(input)
		Expect(err).To(HaveOccurred())
		Expect(ignition).To(BeNil())
	})
})

var _ = Describe("NewJoinControlPlane", func() {
	var input *ControlPlaneJoinInput

	BeforeEach(func() {
		input = &ControlPlaneJoinInput{
			ControlPlaneInput: &cloudinit.ControlPlaneInput{
				BaseUserData: cloudinit.BaseUserData{
					RKE2Version: "v1.21.3+rke2r1",
					WriteFiles: []bootstrapv1.File{
						{
							Path:        "/test/file",
							Content:     "test",
							Permissions: "0644",
						},
					},
					ConfigFile: bootstrapv1.File{
						Path:        "/test/config",
						Content:     "test",
						Permissions: "0644",
					},
				},
			},
			AdditionalIgnition: &bootstrapv1.AdditionalUserData{
				Config: additionalIgnition,
				Strict: true,
			},
		}
	})

	It("should return ignition data for worker", func() {
		ignition, err := NewJoinControlPlane(input)
		Expect(err).ToNot(HaveOccurred())
		Expect(ignition).ToNot(BeNil())
	})

	It("should return error if input is nil", func() {
		input = nil
		ignition, err := NewJoinControlPlane(input)
		Expect(err).To(HaveOccurred())
		Expect(ignition).To(BeNil())
	})

	It("should return error if control plane input is nil", func() {
		input.ControlPlaneInput = nil
		ignition, err := NewJoinControlPlane(input)
		Expect(err).To(HaveOccurred())
		Expect(ignition).To(BeNil())
	})
})

var _ = Describe("NewInitControlPlane", func() {
	var input *ControlPlaneInitInput

	BeforeEach(func() {
		input = &ControlPlaneInitInput{
			ControlPlaneInput: &cloudinit.ControlPlaneInput{
				BaseUserData: cloudinit.BaseUserData{
					RKE2Version: "v1.21.3+rke2r1",
					WriteFiles: []bootstrapv1.File{
						{
							Path:        "/test/file",
							Content:     "test",
							Permissions: "0644",
						},
					},
					ConfigFile: bootstrapv1.File{
						Path:        "/test/config",
						Content:     "test",
						Permissions: "0644",
					},
				},
			},
			AdditionalIgnition: &bootstrapv1.AdditionalUserData{
				Config: additionalIgnition,
				Strict: true,
			},
		}
	})

	It("should return ignition data for worker", func() {
		ignition, err := NewInitControlPlane(input)
		Expect(err).ToNot(HaveOccurred())
		Expect(ignition).ToNot(BeNil())
	})

	It("should return error if input is nil", func() {
		input = nil
		ignition, err := NewInitControlPlane(input)
		Expect(err).To(HaveOccurred())
		Expect(ignition).To(BeNil())
	})

	It("should return error if control plane input is nil", func() {
		input.ControlPlaneInput = nil
		ignition, err := NewInitControlPlane(input)
		Expect(err).To(HaveOccurred())
		Expect(ignition).To(BeNil())
	})
})

var _ = Describe("getControlPlaneRKE2Commands", func() {
	var baseUserData *cloudinit.BaseUserData

	BeforeEach(func() {
		baseUserData = &cloudinit.BaseUserData{
			RKE2Version: "v1.21.3+rke2r1",
		}
	})

	It("should return slice of commands", func() {
		commands, err := getControlPlaneRKE2Commands(baseUserData)
		Expect(err).ToNot(HaveOccurred())
		Expect(commands).To(HaveLen(3))
		Expect(commands).To(ContainElements(fmt.Sprintf(controlPlaneCommand, baseUserData.RKE2Version), serverSystemdServices[0], serverSystemdServices[1]))
	})

	It("should return slice of commands with air gapped", func() {
		baseUserData.AirGapped = true
		commands, err := getControlPlaneRKE2Commands(baseUserData)
		Expect(err).ToNot(HaveOccurred())
		Expect(commands).To(HaveLen(3))
		Expect(commands).To(ContainElements(airGappedControlPlaneCommand, serverSystemdServices[0], serverSystemdServices[1]))
	})

	It("should return error if base userdata is nil", func() {
		baseUserData = nil
		commands, err := getControlPlaneRKE2Commands(baseUserData)
		Expect(err).To(HaveOccurred())
		Expect(commands).To(BeNil())
	})

	It("should return error if rke2 version is not set", func() {
		baseUserData.RKE2Version = ""
		commands, err := getControlPlaneRKE2Commands(baseUserData)
		Expect(err).To(HaveOccurred())
		Expect(commands).To(BeNil())
	})
})

var _ = Describe("getWorkerRKE2Commands", func() {
	var baseUserData *cloudinit.BaseUserData

	BeforeEach(func() {
		baseUserData = &cloudinit.BaseUserData{
			RKE2Version: "v1.21.3+rke2r1",
			AirGapped:   false,
		}
	})

	It("should return slice of commands", func() {
		commands, err := getWorkerRKE2Commands(baseUserData)
		Expect(err).ToNot(HaveOccurred())
		Expect(commands).To(HaveLen(3))
		Expect(commands).To(ContainElements(fmt.Sprintf(workerCommand, baseUserData.RKE2Version), workerSystemdServices[0], workerSystemdServices[1]))
	})

	It("should return slice of commands with air gapped", func() {
		baseUserData.AirGapped = true
		commands, err := getWorkerRKE2Commands(baseUserData)
		Expect(err).ToNot(HaveOccurred())
		Expect(commands).To(HaveLen(3))
		Expect(commands).To(ContainElements(airGappedWorkerCommand, workerSystemdServices[0], workerSystemdServices[1]))
	})

	It("should return error if base userdata is nil", func() {
		baseUserData = nil
		commands, err := getWorkerRKE2Commands(baseUserData)
		Expect(err).To(HaveOccurred())
		Expect(commands).To(BeNil())
	})

	It("should return error if rke2 version is not set", func() {
		baseUserData.RKE2Version = ""
		commands, err := getWorkerRKE2Commands(baseUserData)
		Expect(err).To(HaveOccurred())
		Expect(commands).To(BeNil())
	})
})
