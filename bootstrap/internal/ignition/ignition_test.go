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
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ignition "github.com/coreos/ignition/v2/config/v3_3"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/bootstrap/internal/cloudinit"
)

func TestIgnition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ignition Suite")
}

var flatcarIgnition = `variant: flatcar`

var additionalIgnition = `variant: fcos
version: 1.4.0
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
		ignitionJson, err := NewJoinWorker(input)
		Expect(err).ToNot(HaveOccurred())
		Expect(ignitionJson).ToNot(BeNil())

		ign, reports, err := ignition.Parse(ignitionJson)
		Expect(err).ToNot(HaveOccurred())
		Expect(reports.IsFatal()).To(BeFalse())

		Expect(ign.Storage.Files).To(HaveLen(4))
		Expect(ign.Storage.Files[0].Path).To(Equal("/etc/ssh/sshd_config.d/010-rke2.conf"))
		Expect(ign.Storage.Files[1].Path).To(Equal("/test/file"))
		Expect(ign.Storage.Files[2].Path).To(Equal("/test/config"))
		Expect(ign.Storage.Files[3].Path).To(Equal("/etc/rke2-install.sh"))
	})

	It("should return error if input is nil", func() {
		input = nil
		ignitionJson, err := NewJoinWorker(input)
		Expect(err).To(HaveOccurred())
		Expect(ignitionJson).To(BeNil())
	})

	It("should return error if base userdata is nil", func() {
		input.BaseUserData = nil
		ignitionJson, err := NewJoinWorker(input)
		Expect(err).To(HaveOccurred())
		Expect(ignitionJson).To(BeNil())
	})

	It("should add preparation script with CISEnabled", func() {
		input.CISEnabled = true
		ignitionJson, err := NewJoinWorker(input)
		Expect(err).ToNot(HaveOccurred())
		Expect(ignitionJson).ToNot(BeNil())

		ign, reports, err := ignition.Parse(ignitionJson)
		Expect(err).ToNot(HaveOccurred())
		Expect(reports.IsFatal()).To(BeFalse())

		Expect(ign.Storage.Files).To(HaveLen(4))
		Expect(ign.Storage.Files[0].Path).To(Equal("/etc/ssh/sshd_config.d/010-rke2.conf"))
		Expect(ign.Storage.Files[1].Path).To(Equal("/test/file"))
		Expect(ign.Storage.Files[2].Path).To(Equal("/test/config"))
		Expect(ign.Storage.Files[3].Path).To(Equal("/etc/rke2-install.sh"))

		// Check rke2-install.sh contains the call to rke2-cis-script.sh
		// The ignition file contents is gzipped and base64 encoded, so unpack it first
		scriptContentsEnc := strings.Split(*ign.Storage.Files[3].Contents.Source, ",")[1]
		scriptContentsGzip, err := base64.StdEncoding.DecodeString(scriptContentsEnc)
		Expect(err).ToNot(HaveOccurred())
		reader := bytes.NewReader(scriptContentsGzip)
		gzreader, err := gzip.NewReader(reader)
		Expect(err).ToNot(HaveOccurred())
		scriptContents, err := io.ReadAll(gzreader)
		Expect(err).ToNot(HaveOccurred())
		Expect(scriptContents).To(ContainSubstring("/opt/rke2-cis-script.sh"))
	})
	It("should render rke2-install without semanage for flatcar", func() {
		input.AdditionalIgnition = &bootstrapv1.AdditionalUserData{
			Config: flatcarIgnition,
			Strict: true,
		}
		ignitionJson, err := NewJoinWorker(input)
		Expect(err).ToNot(HaveOccurred())
		Expect(ignitionJson).ToNot(BeNil())

		ign, reports, err := ignition.Parse(ignitionJson)
		Expect(err).ToNot(HaveOccurred())
		Expect(reports.IsFatal()).To(BeFalse())

		scriptContentsEnc := strings.Split(*ign.Storage.Files[3].Contents.Source, ",")[1]
		scriptContentsGzip, err := base64.StdEncoding.DecodeString(scriptContentsEnc)
		Expect(err).ToNot(HaveOccurred())
		reader := bytes.NewReader(scriptContentsGzip)
		gzreader, err := gzip.NewReader(reader)
		Expect(err).ToNot(HaveOccurred())
		scriptContents, err := io.ReadAll(gzreader)
		Expect(err).ToNot(HaveOccurred())
		Expect(scriptContents).ToNot(ContainSubstring("semanage"))
	})
})

var _ = Describe("NewJoinControlPlane", func() {
	var input *ControlPlaneInput

	BeforeEach(func() {
		input = &ControlPlaneInput{
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

	It("should return ignition data for control plane", func() {
		ignitionJson, err := NewJoinControlPlane(input)
		Expect(err).ToNot(HaveOccurred())
		Expect(ignitionJson).ToNot(BeNil())
	})

	It("should return error if input is nil", func() {
		input = nil
		ignitionJson, err := NewJoinControlPlane(input)
		Expect(err).To(HaveOccurred())
		Expect(ignitionJson).To(BeNil())
	})

	It("should return error if control plane input is nil", func() {
		input.ControlPlaneInput = nil
		ignitionJson, err := NewJoinControlPlane(input)
		Expect(err).To(HaveOccurred())
		Expect(ignitionJson).To(BeNil())
	})
	It("should render rke2-install without semanage for flatcar", func() {
		input.AdditionalIgnition = &bootstrapv1.AdditionalUserData{
			Config: flatcarIgnition,
			Strict: true,
		}
		ignitionJson, err := NewJoinControlPlane(input)
		Expect(err).ToNot(HaveOccurred())
		Expect(ignitionJson).ToNot(BeNil())

		ign, reports, err := ignition.Parse(ignitionJson)
		Expect(err).ToNot(HaveOccurred())
		Expect(reports.IsFatal()).To(BeFalse())

		scriptContentsEnc := strings.Split(*ign.Storage.Files[3].Contents.Source, ",")[1]
		scriptContentsGzip, err := base64.StdEncoding.DecodeString(scriptContentsEnc)
		Expect(err).ToNot(HaveOccurred())
		reader := bytes.NewReader(scriptContentsGzip)
		gzreader, err := gzip.NewReader(reader)
		Expect(err).ToNot(HaveOccurred())
		scriptContents, err := io.ReadAll(gzreader)
		Expect(err).ToNot(HaveOccurred())
		Expect(scriptContents).ToNot(ContainSubstring("semanage"))
	})
})

var _ = Describe("NewInitControlPlane", func() {
	var input *ControlPlaneInput

	BeforeEach(func() {
		input = &ControlPlaneInput{
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

	It("should return ignition data for control plane", func() {
		ignitionJson, err := NewInitControlPlane(input)
		Expect(err).ToNot(HaveOccurred())
		Expect(ignitionJson).ToNot(BeNil())
	})

	It("should return error if input is nil", func() {
		input = nil
		ignitionJson, err := NewInitControlPlane(input)
		Expect(err).To(HaveOccurred())
		Expect(ignitionJson).To(BeNil())
	})

	It("should return error if control plane input is nil", func() {
		input.ControlPlaneInput = nil
		ignitionJson, err := NewInitControlPlane(input)
		Expect(err).To(HaveOccurred())
		Expect(ignitionJson).To(BeNil())
	})
})

var _ = Describe("getControlPlaneRKE2Commands", func() {
	var baseUserData *cloudinit.BaseUserData

	BeforeEach(func() {
		baseUserData = &cloudinit.BaseUserData{
			RKE2Version: "v1.21.3+rke2r1",
		}
	})

	It("should return slice of control plane commands", func() {
		commands, err := getControlPlaneRKE2Commands(baseUserData)
		Expect(err).ToNot(HaveOccurred())
		Expect(commands).To(HaveLen(7))
		Expect(commands).To(ContainElements(fmt.Sprintf(controlPlaneCommand, baseUserData.RKE2Version), serverDeployCommands[0], serverDeployCommands[1]))
	})

	It("should return slice of control plane commands with air gapped", func() {
		baseUserData.AirGapped = true
		commands, err := getControlPlaneRKE2Commands(baseUserData)
		Expect(err).ToNot(HaveOccurred())
		Expect(commands).To(HaveLen(7))
		Expect(commands).To(ContainElements(airGappedControlPlaneCommand, serverDeployCommands[0], serverDeployCommands[1]))
	})

	It("should return slice of control plane commands with air gapped and checksum verify", func() {
		baseUserData.AirGapped = true
		baseUserData.AirGappedChecksum = "abcd"
		commands, err := getControlPlaneRKE2Commands(baseUserData)
		Expect(err).ToNot(HaveOccurred())
		Expect(commands).To(HaveLen(8))
		Expect(commands).To(ContainElements(fmt.Sprintf(airGappedChecksumCommand, "abcd"), airGappedControlPlaneCommand, serverDeployCommands[0], serverDeployCommands[1]))
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

	It("should return slice of worker commands", func() {
		commands, err := getWorkerRKE2Commands(baseUserData)
		Expect(err).ToNot(HaveOccurred())
		Expect(commands).To(HaveLen(7))
		Expect(commands).To(ContainElements(fmt.Sprintf(workerCommand, baseUserData.RKE2Version), workerDeployCommands[0], workerDeployCommands[1]))
	})

	It("should return slice of worker commands with air gapped", func() {
		baseUserData.AirGapped = true
		commands, err := getWorkerRKE2Commands(baseUserData)
		Expect(err).ToNot(HaveOccurred())
		Expect(commands).To(HaveLen(7))
		Expect(commands).To(ContainElements(airGappedWorkerCommand, workerDeployCommands[0], workerDeployCommands[1]))
	})

	It("should return slice of worker commands with air gapped and checksum verify", func() {
		baseUserData.AirGapped = true
		baseUserData.AirGappedChecksum = "abcd"
		commands, err := getWorkerRKE2Commands(baseUserData)
		Expect(err).ToNot(HaveOccurred())
		Expect(commands).To(HaveLen(8))
		Expect(commands).To(ContainElements(fmt.Sprintf(airGappedChecksumCommand, "abcd"), workerDeployCommands[0], workerDeployCommands[1]))
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
