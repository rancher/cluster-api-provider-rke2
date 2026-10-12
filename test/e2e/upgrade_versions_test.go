//go:build e2e
// +build e2e

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

package e2e

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
)

func testUpgradeConfig(devVersion string) *clusterctl.E2EConfig {
	return &clusterctl.E2EConfig{
		Providers: []clusterctl.ProviderConfig{
			{Name: coreProviderName, Versions: []clusterctl.ProviderVersionSource{
				{Name: capiPreviousMinorPlaceholder, Value: "https://example.com/" + capiPreviousMinorPlaceholder + "/core-components.yaml"},
				{Name: "v1.14.2", Value: "https://example.com/v1.14.2/core-components.yaml"},
			}},
			{Name: dockerProviderName, Versions: []clusterctl.ProviderVersionSource{
				{Name: capiPreviousMinorPlaceholder, Value: "https://example.com/" + capiPreviousMinorPlaceholder + "/infra.yaml"},
				{Name: "v1.14.2", Value: "https://example.com/v1.14.2/infra.yaml"},
			}},
			{Name: bootstrapProviderName, Versions: []clusterctl.ProviderVersionSource{
				{Name: rke2PreviousMinorPlaceholder, Value: "https://example.com/" + rke2PreviousMinorPlaceholder + "/bootstrap-components.yaml"},
				{Name: devVersion, Value: "../../../bootstrap/config/default"},
			}},
		},
	}
}

func TestApplyUpgradeFromVersions(t *testing.T) {
	g := NewWithT(t)

	config := testUpgradeConfig("v0.27.99")
	g.Expect(applyUpgradeFromVersions(config, upgradeVersions{RKE2: "v0.26.0", CAPI: "v1.13.5"})).To(Succeed())

	g.Expect(config.Providers[0].Versions).To(HaveLen(2))
	g.Expect(config.Providers[0].Versions[0].Name).To(Equal("v1.13.5"))
	g.Expect(config.Providers[0].Versions[0].Value).To(Equal("https://example.com/v1.13.5/core-components.yaml"))
	g.Expect(config.Providers[2].Versions[0].Name).To(Equal("v0.26.0"))
	g.Expect(config.Providers[2].Versions[0].Value).To(Equal("https://example.com/v0.26.0/bootstrap-components.yaml"))

	to, err := upgradeToVersions(config)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(to).To(Equal(upgradeVersions{RKE2: "v0.27.99", CAPI: "v1.14.2"}))
}

func TestApplyUpgradeFromVersionsDeduplicates(t *testing.T) {
	g := NewWithT(t)

	config := testUpgradeConfig("v0.27.99")
	g.Expect(applyUpgradeFromVersions(config, upgradeVersions{RKE2: "v0.26.0", CAPI: "v1.14.2"})).To(Succeed())

	g.Expect(config.Providers[0].Versions).To(HaveLen(1))
	g.Expect(config.Providers[1].Versions).To(HaveLen(1))
	g.Expect(config.Providers[0].Versions[0].Name).To(Equal("v1.14.2"))
}

func TestApplyUpgradeFromVersionsRejectsCAPIDowngrade(t *testing.T) {
	g := NewWithT(t)

	config := testUpgradeConfig("v0.27.99")
	g.Expect(applyUpgradeFromVersions(config, upgradeVersions{RKE2: "v0.26.0", CAPI: "v1.15.0"})).ToNot(Succeed())
}

func TestPreviousMinorOf(t *testing.T) {
	g := NewWithT(t)

	g.Expect(previousMinorOf("v0.27.99")).To(Equal("v0.26"))
	g.Expect(previousMinorOf("v1.10.99")).To(Equal("v1.9"))

	_, err := previousMinorOf("v1.0.99")
	g.Expect(err).To(HaveOccurred())
}

func TestDevProviderVersionRequiresOneDevVersion(t *testing.T) {
	g := NewWithT(t)

	config := testUpgradeConfig("v0.27.0")
	_, err := devProviderVersion(config)
	g.Expect(err).To(HaveOccurred())
}

func TestCAPIVersionFromGoMod(t *testing.T) {
	g := NewWithT(t)

	g.Expect(capiVersionFromGoMod([]byte("module x\n\nrequire (\n\tsigs.k8s.io/cluster-api v1.14.2\n\tsigs.k8s.io/cluster-api/test v1.14.2\n)\n"))).
		To(Equal("v1.14.2"))

	_, err := capiVersionFromGoMod([]byte("module x\n"))
	g.Expect(err).To(HaveOccurred())
}

func TestGoProxyURL(t *testing.T) {
	g := NewWithT(t)

	for goproxy, want := range map[string]string{
		"":                                "https://proxy.golang.org",
		"https://proxy.golang.org,direct": "https://proxy.golang.org",
		"goproxy.example.com/|direct":     "https://goproxy.example.com",
	} {
		t.Setenv("GOPROXY", goproxy)
		g.Expect(goProxyURL()).To(Equal(want), "GOPROXY=%q", goproxy)
	}

	t.Setenv("GOPROXY", "off")
	_, err := goProxyURL()
	g.Expect(err).To(HaveOccurred())
}

// TestResolveUpgradeFromVersionsOnline queries the Go module proxy and GitHub releases.
// Run with: E2E_RESOLVE_ONLINE=1 go test -tags e2e -run TestResolveUpgradeFromVersionsOnline ./test/e2e/
func TestResolveUpgradeFromVersionsOnline(t *testing.T) {
	if os.Getenv("E2E_RESOLVE_ONLINE") == "" {
		t.Skip("set E2E_RESOLVE_ONLINE=1 to query the Go module proxy and GitHub releases")
	}

	t.Setenv(UpgradeFromVersion, "")
	t.Setenv(UpgradeFromCAPIVersion, "")

	for _, devVersion := range []string{"v0.26.99", "v0.27.99", "v0.28.99"} {
		from, err := resolveUpgradeFromVersions(context.Background(), testUpgradeConfig(devVersion))
		if err != nil {
			t.Fatalf("%s: %v", devVersion, err)
		}

		t.Logf("%s upgrades from CAPRKE2 %s with Cluster API %s", devVersion, from.RKE2, from.CAPI)
	}
}

func TestResolveUpgradeFromVersionsOverrides(t *testing.T) {
	g := NewWithT(t)

	t.Setenv(UpgradeFromVersion, "v0.26.0")
	t.Setenv(UpgradeFromCAPIVersion, "v1.14.2")

	from, err := resolveUpgradeFromVersions(context.Background(), testUpgradeConfig("v0.27.99"))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(from).To(Equal(upgradeVersions{RKE2: "v0.26.0", CAPI: "v1.14.2"}))
}
