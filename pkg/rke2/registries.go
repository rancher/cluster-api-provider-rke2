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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1alpha2"
	bsutil "github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/util"
)

const (
	// DefaultRKE2RegistriesLocation is the default location for the registries.yaml file.
	DefaultRKE2RegistriesLocation string = "/etc/rancher/rke2/registries.yaml"

	registryCertsPath string = "/etc/rancher/rke2/tls"
)

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

	for configName, regConfig := range rke2ConfigRegistry.Registry.Configs {
		tlsSecret := corev1.Secret{}
		authSecret := corev1.Secret{}

		err := rke2ConfigRegistry.Client.Get(
			rke2ConfigRegistry.Ctx,
			types.NamespacedName{
				Name:      regConfig.TLS.TLSConfigSecret.Name,
				Namespace: regConfig.TLS.TLSConfigSecret.Namespace,
			},
			&tlsSecret,
		)
		if err != nil {
			rke2ConfigRegistry.Logger.Error(err, "TLS Config Secret for the registry was not found!")

			return &Registry{}, []bootstrapv1.File{}, err
		}

		for _, secretEntry := range []string{"tls.crt", "tls.key", "ca.crt"} {
			if tlsSecret.Data[secretEntry] == nil {
				rke2ConfigRegistry.Logger.Error(err, "TLS Config Secret for the registry is missing entries!", "secret-missing-entry", secretEntry)

				return &Registry{}, []bootstrapv1.File{}, err
			}

			files = append(files, bootstrapv1.File{
				Path:    registryCertsPath + "/" + secretEntry,
				Content: string(tlsSecret.Data[secretEntry]),
			})
		}

		err = rke2ConfigRegistry.Client.Get(
			rke2ConfigRegistry.Ctx,
			types.NamespacedName{
				Name:      regConfig.AuthSecret.Name,
				Namespace: regConfig.AuthSecret.Namespace,
			},
			&authSecret,
		)

		if err != nil {
			rke2ConfigRegistry.Logger.Error(err, "Auth Config Secret for the registry was not found!")

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

		registry.Configs = make(map[string]RegistryConfig)
		registry.Configs[configName] = RegistryConfig{
			TLS: &TLSConfig{
				InsecureSkipVerify: regConfig.TLS.InsecureSkipVerify,
				CAFile:             registryCertsPath + "/" + "ca.crt",
				CertFile:           registryCertsPath + "/" + "tls.crt",
				KeyFile:            registryCertsPath + "/" + "tls.key",
			},
			Auth: authData,
		}
	}

	return registry, files, nil
}
