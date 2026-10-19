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
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"

	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
)

const (
	// rke2PreviousMinorPlaceholder is replaced with the latest release of the previous CAPRKE2 minor.
	rke2PreviousMinorPlaceholder = "{rke2-previous-minor}"
	// capiPreviousMinorPlaceholder is replaced with the Cluster API version required by that release.
	capiPreviousMinorPlaceholder = "{capi-previous-minor}"

	// UpgradeFromVersion pins the CAPRKE2 version the upgrade test starts from (e.g. v0.26.0).
	UpgradeFromVersion = "E2E_UPGRADE_FROM_VERSION"
	// UpgradeFromCAPIVersion pins the Cluster API version the upgrade test starts from (e.g. v1.14.2).
	UpgradeFromCAPIVersion = "E2E_UPGRADE_FROM_CAPI_VERSION"

	caprke2Module    = "github.com/rancher/cluster-api-provider-rke2"
	capiModule       = "sigs.k8s.io/cluster-api"
	defaultGoProxy   = "https://proxy.golang.org"
	devVersionSuffix = ".99"

	coreProviderName      = "cluster-api"
	dockerProviderName    = "docker"
	bootstrapProviderName = "rke2-bootstrap"
	cpProviderName        = "rke2-control-plane"
)

// upgradeVersions holds the provider versions used by the upgrade test.
type upgradeVersions struct {
	RKE2 string
	CAPI string
}

// resolveUpgradeFromVersions returns the versions the upgrade test starts from: the latest release
// of the previous CAPRKE2 minor (a pre-release if there is no GA yet) and the Cluster API version
// in that release's go.mod. The environment overrides take precedence.
func resolveUpgradeFromVersions(ctx context.Context, config *clusterctl.E2EConfig) (upgradeVersions, error) {
	from := upgradeVersions{
		RKE2: os.Getenv(UpgradeFromVersion),
		CAPI: os.Getenv(UpgradeFromCAPIVersion),
	}

	if from.RKE2 == "" {
		devVersion, err := devProviderVersion(config)
		if err != nil {
			return from, err
		}

		previousMinor, err := previousMinorOf(devVersion)
		if err != nil {
			return from, err
		}

		from.RKE2, err = resolveLatestRelease(ctx, caprke2Module, previousMinor)
		if err != nil {
			return from, err
		}
	}

	if from.CAPI == "" {
		var err error

		from.CAPI, err = capiVersionOf(ctx, from.RKE2)
		if err != nil {
			return from, err
		}
	}

	for _, v := range []string{from.RKE2, from.CAPI} {
		if !semver.IsValid(v) {
			return from, fmt.Errorf("invalid upgrade starting version %q", v)
		}
	}

	return from, nil
}

// applyUpgradeFromVersions substitutes the placeholders in the provider versions of the config.
// A placeholder entry is dropped when the resolved version is already listed for that provider.
func applyUpgradeFromVersions(config *clusterctl.E2EConfig, from upgradeVersions) error {
	to, err := upgradeToVersions(config)
	if err != nil {
		return err
	}

	if semver.Compare(from.CAPI, to.CAPI) > 0 {
		return fmt.Errorf("cluster API %s required by CAPRKE2 %s is newer than the upgrade target %s: add %s to %s and %s in the e2e config",
			from.CAPI, from.RKE2, to.CAPI, from.CAPI, coreProviderName, dockerProviderName)
	}

	replacer := strings.NewReplacer(
		rke2PreviousMinorPlaceholder, from.RKE2,
		capiPreviousMinorPlaceholder, from.CAPI,
	)

	for i := range config.Providers {
		provider := &config.Providers[i]

		existing := map[string]bool{}

		for _, v := range provider.Versions {
			if !hasPlaceholder(v.Name) {
				existing[v.Name] = true
			}
		}

		versions := provider.Versions[:0]
		for _, v := range provider.Versions {
			if hasPlaceholder(v.Name) {
				v.Name = replacer.Replace(v.Name)
				v.Value = replacer.Replace(v.Value)

				if existing[v.Name] {
					continue
				}
			}

			versions = append(versions, v)
		}

		provider.Versions = versions
	}

	return nil
}

// upgradeToVersions returns the versions the upgrade test upgrades to: the development version of
// CAPRKE2 and the newest Cluster API version in the config.
func upgradeToVersions(config *clusterctl.E2EConfig) (upgradeVersions, error) {
	rke2, err := devProviderVersion(config)
	if err != nil {
		return upgradeVersions{}, err
	}

	capi := ""

	for _, provider := range config.Providers {
		if provider.Name != coreProviderName {
			continue
		}

		for _, v := range provider.Versions {
			if semver.IsValid(v.Name) && (capi == "" || semver.Compare(v.Name, capi) > 0) {
				capi = v.Name
			}
		}
	}

	if capi == "" {
		return upgradeVersions{}, fmt.Errorf("no valid %s version in the e2e config", coreProviderName)
	}

	return upgradeVersions{RKE2: rke2, CAPI: capi}, nil
}

// devProviderVersion returns the single development (vX.Y.99) version of the rke2-bootstrap provider.
func devProviderVersion(config *clusterctl.E2EConfig) (string, error) {
	devVersions := []string{}

	for _, provider := range config.Providers {
		if provider.Name != bootstrapProviderName {
			continue
		}

		for _, v := range provider.Versions {
			if semver.IsValid(v.Name) && strings.HasSuffix(v.Name, devVersionSuffix) {
				devVersions = append(devVersions, v.Name)
			}
		}
	}

	if len(devVersions) != 1 {
		return "", fmt.Errorf("expected exactly one %s version ending in %s in the e2e config, found %v",
			bootstrapProviderName, devVersionSuffix, devVersions)
	}

	return devVersions[0], nil
}

// previousMinorOf returns the previous minor release series, e.g. v0.27.99 -> v0.26.
func previousMinorOf(version string) (string, error) {
	var major, minor int
	if _, err := fmt.Sscanf(semver.MajorMinor(version), "v%d.%d", &major, &minor); err != nil {
		return "", fmt.Errorf("parsing version %q: %w", version, err)
	}

	if minor == 0 {
		return "", fmt.Errorf("cannot derive the previous minor of %s", version)
	}

	return fmt.Sprintf("v%d.%d", major, minor-1), nil
}

// resolveLatestRelease returns the latest GA patch of a release series, or its latest pre-release
// when the series has no GA yet.
func resolveLatestRelease(ctx context.Context, module, series string) (string, error) {
	version, err := clusterctl.ResolveRelease(ctx, fmt.Sprintf("go://%s@%s", module, series))
	if err != nil {
		var preErr error

		version, preErr = clusterctl.ResolveRelease(ctx, fmt.Sprintf("go://%s@latest-%s", module, series))
		if preErr != nil {
			return "", fmt.Errorf("resolving the latest %s release of %s: %w", series, module, preErr)
		}
	}

	return "v" + strings.TrimPrefix(version, "v"), nil
}

// capiVersionOf returns the Cluster API version required by a CAPRKE2 release, read from its go.mod.
func capiVersionOf(ctx context.Context, rke2Version string) (string, error) {
	proxy, err := goProxyURL()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/%s/@v/%s.mod", proxy, caprke2Module, rke2Version)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: %s", url, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", url, err)
	}

	return capiVersionFromGoMod(data)
}

func capiVersionFromGoMod(data []byte) (string, error) {
	mod, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		return "", fmt.Errorf("parsing go.mod: %w", err)
	}

	for _, r := range mod.Require {
		if r.Mod.Path == capiModule {
			return r.Mod.Version, nil
		}
	}

	return "", fmt.Errorf("%s not found in go.mod", capiModule)
}

// goProxyURL returns the first proxy URL in GOPROXY, defaulting to proxy.golang.org.
func goProxyURL() (string, error) {
	for _, entry := range strings.FieldsFunc(os.Getenv("GOPROXY"), func(r rune) bool { return r == ',' || r == '|' }) {
		entry = strings.TrimSpace(entry)

		switch entry {
		case "":
			continue
		case "off", "direct":
			return "", fmt.Errorf("GOPROXY=%s cannot resolve releases: set %s and %s", entry, UpgradeFromVersion, UpgradeFromCAPIVersion)
		}

		if !strings.Contains(entry, "://") {
			entry = "https://" + entry
		}

		return strings.TrimSuffix(entry, "/"), nil
	}

	return defaultGoProxy, nil
}

func hasPlaceholder(s string) bool {
	return strings.Contains(s, rke2PreviousMinorPlaceholder) || strings.Contains(s, capiPreviousMinorPlaceholder)
}

// providerVersion formats a provider reference for clusterctl, e.g. rke2-bootstrap:v0.26.0.
func providerVersion(name, version string) string {
	return name + ":" + version
}
