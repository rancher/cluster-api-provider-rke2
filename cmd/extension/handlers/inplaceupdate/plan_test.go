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
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
)

func TestBuildUpgradePlan_ControlPlane(t *testing.T) {
	g := NewWithT(t)

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "cp-1",
			Labels:    map[string]string{clusterv1.MachineControlPlaneLabel: ""},
		},
		Spec: clusterv1.MachineSpec{Version: "v1.30.2+rke2r1"},
	}

	p, err := buildUpgradePlan(machine, nil)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(p.OneTimeInstructions).To(HaveLen(2))

	install := p.OneTimeInstructions[0]
	g.Expect(install.Image).To(Equal("rancher/system-agent-installer-rke2:v1.30.2-rke2r1"))
	g.Expect(install.Env).NotTo(ContainElement("INSTALL_RKE2_TYPE=agent"))

	restart := p.OneTimeInstructions[1]
	g.Expect(restart.Command).To(Equal("systemctl"))
	g.Expect(restart.Args).To(Equal([]string{systemctlRestartArg, rke2ServerServiceName}))
}

func TestBuildUpgradePlan_Worker(t *testing.T) {
	g := NewWithT(t)

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "worker-1"},
		Spec:       clusterv1.MachineSpec{Version: "v1.30.2+rke2r1"},
	}

	p, err := buildUpgradePlan(machine, nil)
	g.Expect(err).NotTo(HaveOccurred())

	install := p.OneTimeInstructions[0]
	g.Expect(install.Env).To(ContainElement("INSTALL_RKE2_TYPE=agent"))

	restart := p.OneTimeInstructions[1]
	g.Expect(restart.Args).To(Equal([]string{systemctlRestartArg, rke2AgentServiceName}))
}

func TestBuildUpgradePlan_MissingVersion(t *testing.T) {
	g := NewWithT(t)

	machine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "m1"}}

	_, err := buildUpgradePlan(machine, nil)
	g.Expect(err).To(HaveOccurred())
}

func TestConvertFiles(t *testing.T) {
	g := NewWithT(t)

	files, err := convertFiles([]bootstrapv1.File{
		{Path: "/plain", Content: "hello", Permissions: "0644"},
		{Path: "/b64", Content: base64.StdEncoding.EncodeToString([]byte("hello")), Encoding: bootstrapv1.Base64},
		{Path: "/gzb64", Content: gzipBase64(t, "hello"), Encoding: bootstrapv1.GzipBase64},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(files).To(HaveLen(3))

	for _, f := range files {
		g.Expect(f.Content).To(Equal(base64.StdEncoding.EncodeToString([]byte("hello"))), f.Path)
	}
	g.Expect(files[0].Permissions).To(Equal("0644"))
}

func TestConvertFiles_ContentFromUnsupported(t *testing.T) {
	g := NewWithT(t)

	_, err := convertFiles([]bootstrapv1.File{
		{Path: "/secret-backed", ContentFrom: &bootstrapv1.FileSource{Secret: &bootstrapv1.FileSourceRef{Name: "s1", Key: "k1"}}},
	})
	g.Expect(err).To(HaveOccurred())
}

func gzipBase64(t *testing.T, s string) string {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write([]byte(s))
	if err != nil {
		t.Fatalf("failed to gzip content: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
