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

package rke2

import (
	"fmt"
	"regexp"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	bsutil "github.com/rancher/cluster-api-provider-rke2/pkg/util"
)

const (
	// DefaultRKE2RegistriesLocation is the default location for the registries.yaml file.
	DefaultRKE2RegistriesLocation string = "/etc/rancher/rke2/registries.yaml"

	registryCertsPath string = "/etc/rancher/rke2/tls"
	cacert            string = "ca.crt"
	tlskey            string = "tls.key"
	tlscert           string = "tls.crt"
)

// unsafeRegistryPathChars matches the characters of a registry name that are
// not kept in the name of the directory holding that registry's TLS files.
var unsafeRegistryPathChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// registryCertsDir returns the directory for the TLS files of one registry, so
// the files of different registries do not overwrite each other.
func registryCertsDir(configName string) string {
	return registryCertsPath + "/" + unsafeRegistryPathChars.ReplaceAllString(configName, "_")
}

// GenerateRegistries generates the registries.yaml file and the corresponding
// files for the TLS certificates.
func GenerateRegistries(rke2ConfigRegistry RegistryScope) (*Registry, []bootstrapv1.File, error) {
	registry := &Registry{}
	files := []bootstrapv1.File{}
	registry.Mirrors = make(map[string]Mirror)

	for mirrorName, mirror := range rke2ConfigRegistry.Registry.Mirrors {
		registry.Mirrors[mirrorName] = Mirror{
			Endpoint: mirror.Endpoint,
			Rewrite:  mirror.Rewrite,
		}
	}

	registry.Configs = make(map[string]RegistryConfig)

	// Walk the configs in a fixed order, so the generated files come out in
	// the same order every time.
	configNames := make([]string, 0, len(rke2ConfigRegistry.Registry.Configs))
	for configName := range rke2ConfigRegistry.Registry.Configs {
		configNames = append(configNames, configName)
	}

	sort.Strings(configNames)

	certsDirs := make(map[string]string, len(configNames))

	for _, configName := range configNames {
		regConfig := rke2ConfigRegistry.Registry.Configs[configName]
		registryConfig := RegistryConfig{}

		if regConfig.TLS != (bootstrapv1.TLSConfig{}) {
			tlsSecret := corev1.Secret{}

			err := rke2ConfigRegistry.Client.Get(
				rke2ConfigRegistry.Ctx,
				types.NamespacedName{
					Name:      regConfig.TLS.TLSConfigSecret.Name,
					Namespace: regConfig.TLS.TLSConfigSecret.Namespace,
				},
				&tlsSecret,
			)
			if err != nil {
				if apierrors.IsNotFound(err) {
					rke2ConfigRegistry.Logger.Error(err, "TLS Secret for the registry was not found!")
				} else {
					rke2ConfigRegistry.Logger.Error(err, "Error fetching TLS Secret")
				}

				return &Registry{}, []bootstrapv1.File{}, err
			}

			registryConfig.TLS = &TLSConfig{}

			certsDir := registryCertsDir(configName)
			if other, ok := certsDirs[certsDir]; ok {
				err := fmt.Errorf("registries %q and %q would write their TLS files to the same directory %s", other, configName, certsDir)
				rke2ConfigRegistry.Logger.Error(err, "Conflicting registry names")

				return &Registry{}, []bootstrapv1.File{}, err
			}

			certsDirs[certsDir] = configName

			for _, secretEntry := range []string{tlscert, tlskey, cacert} {
				if tlsSecret.Data[secretEntry] != nil {
					filePath := certsDir + "/" + secretEntry
					files = append(files, bootstrapv1.File{
						Path:    filePath,
						Content: string(tlsSecret.Data[secretEntry]),
					})

					switch secretEntry {
					case tlscert:
						registryConfig.TLS.CertFile = filePath
					case tlskey:
						registryConfig.TLS.KeyFile = filePath
					case cacert:
						registryConfig.TLS.CAFile = filePath
					}
				}
			}

			if regConfig.TLS.InsecureSkipVerify {
				registryConfig.TLS.InsecureSkipVerify = regConfig.TLS.InsecureSkipVerify
			}
		}

		if regConfig.AuthSecret != (corev1.ObjectReference{}) {
			authSecret := corev1.Secret{}

			err := rke2ConfigRegistry.Client.Get(
				rke2ConfigRegistry.Ctx,
				types.NamespacedName{
					Name:      regConfig.AuthSecret.Name,
					Namespace: regConfig.AuthSecret.Namespace,
				},
				&authSecret,
			)
			if err != nil {
				if apierrors.IsNotFound(err) {
					rke2ConfigRegistry.Logger.Error(err, "AuthSecret for the registry was not found!")
				} else {
					rke2ConfigRegistry.Logger.Error(err, "Error fetching AuthSecret")
				}

				return &Registry{}, []bootstrapv1.File{}, err
			}

			isBasicAuth := authSecret.Data["username"] != nil && authSecret.Data["password"] != nil
			isTokenAuth := authSecret.Data["identity-token"] != nil

			ok := isBasicAuth || isTokenAuth

			if !ok {
				rke2ConfigRegistry.Logger.Error(
					err,
					"Auth Secret for the registry is missing entries! Possible entries are: (\"username\" AND \"password\") OR \"identity-token\" ",
					"secret-entries", bsutil.GetMapKeysAsString(authSecret.Data))

				return &Registry{}, []bootstrapv1.File{}, err
			}

			authData := &AuthConfig{}
			if isBasicAuth {
				authData.Username = string(authSecret.Data["username"])
				authData.Password = string(authSecret.Data["password"])
			}

			if isTokenAuth {
				authData.IdentityToken = string(authSecret.Data["identity-token"])
			}

			registryConfig.Auth = authData
		}

		registry.Configs[configName] = registryConfig
	}

	return registry, files, nil
}
