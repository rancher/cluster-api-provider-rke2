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
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
	planapi "github.com/rancher/rancher/pkg/plan"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
)

// installerImageRepository carries the RKE2 install script for a given version. This mirrors the
// convention used by Rancher's own RKE2 planner (rancher/pkg/capr/planner.getInstallerImage).
const installerImageRepository = "rancher/system-agent-installer-rke2"

const (
	installInstructionName = "rke2-upgrade-install"
	restartInstructionName = "rke2-upgrade-restart"
	rke2ServerServiceName  = "rke2-server"
	rke2AgentServiceName   = "rke2-agent"
	systemctlRestartArg    = "restart"
)

// buildUpgradePlan returns the system-agent Plan that performs an in-place RKE2 binary swap and
// service restart for the given Machine's desired Kubernetes version, and delivers any desired
// RKE2ConfigSpec.Files mirroring the exact set of fields DoCanUpdateMachine claims it can
// absorb (Version and RKE2ConfigSpec.Files).
func buildUpgradePlan(machine *clusterv1.Machine, files []bootstrapv1.File) (planapi.Plan, error) {
	version := machine.Spec.Version
	if version == "" {
		return planapi.Plan{}, errors.Errorf("machine %s/%s has no desired version set", machine.Namespace, machine.Name)
	}

	planFiles, err := convertFiles(files)
	if err != nil {
		return planapi.Plan{}, errors.Wrapf(err, "machine %s/%s", machine.Namespace, machine.Name)
	}

	_, isControlPlane := machine.Labels[clusterv1.MachineControlPlaneLabel]

	serviceName := rke2AgentServiceName

	var installEnv []string

	if isControlPlane {
		serviceName = rke2ServerServiceName
	} else {
		installEnv = append(installEnv, "INSTALL_RKE2_TYPE=agent")
	}

	image := fmt.Sprintf("%s:%s", installerImageRepository, strings.ReplaceAll(version, "+", "-"))

	return planapi.Plan{
		Files: planFiles,
		OneTimeInstructions: []planapi.OneTimeInstruction{
			{
				CommonInstruction: planapi.CommonInstruction{
					Name:    installInstructionName,
					Image:   image,
					Command: "sh",
					Args:    []string{"-c", "run.sh"},
					Env:     installEnv,
				},
			},
			{
				CommonInstruction: planapi.CommonInstruction{
					Name:    restartInstructionName,
					Command: "systemctl",
					Args:    []string{systemctlRestartArg, serviceName},
				},
			},
		},
	}, nil
}

// convertFiles translates RKE2ConfigSpec.Files into system-agent Plan.Files. planapi.File.Content
// is always plain base64 of the final raw bytes (system-agent writes it via a base64 decode only,
// with no notion of gzip), so each bootstrapv1.File's Encoding is resolved to raw bytes first and
// then re-encoded as base64.
//
// ContentFrom (Secret/ConfigMap-sourced content) is not supported yet: resolving it requires
// additional API reads this builder does not perform, so such files fail loudly here rather than
// silently dropping their content.
func convertFiles(files []bootstrapv1.File) ([]planapi.File, error) {
	if len(files) == 0 {
		return nil, nil
	}

	converted := make([]planapi.File, 0, len(files))

	for _, f := range files {
		if f.ContentFrom != nil {
			return nil, errors.Errorf("file %s uses contentFrom, which in-place update plans do not support yet", f.Path)
		}

		raw, err := decodeFileContent(f.Content, f.Encoding)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode content for file %s", f.Path)
		}

		converted = append(converted, planapi.File{
			Path:        f.Path,
			Permissions: f.Permissions,
			Content:     base64.StdEncoding.EncodeToString(raw),
		})
	}

	return converted, nil
}

// decodeFileContent resolves a bootstrapv1.File's Content/Encoding pair into raw bytes.
func decodeFileContent(content string, encoding bootstrapv1.Encoding) ([]byte, error) {
	switch encoding {
	case "":
		return []byte(content), nil
	case bootstrapv1.Base64:
		return base64.StdEncoding.DecodeString(content)
	case bootstrapv1.Gzip:
		return gunzip([]byte(content))
	case bootstrapv1.GzipBase64:
		gz, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, err
		}

		return gunzip(gz)
	default:
		return nil, errors.Errorf("unsupported file encoding %q", encoding)
	}
}

func gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}
