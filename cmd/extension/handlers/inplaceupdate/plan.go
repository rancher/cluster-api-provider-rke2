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
	"strconv"
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
//
// Air-gapped machines are not supported yet: the installer image approach used here requires
// registry access, so an air-gapped machine would only discover the failure deep inside run.sh.
// Failing here instead gives CAPI a clear, immediate Failure rather than a silent hang/timeout.
func buildUpgradePlan(machine *clusterv1.Machine, files []bootstrapv1.File, agentConfig bootstrapv1.RKE2AgentConfig) (planapi.Plan, error) {
	version := machine.Spec.Version
	if version == "" {
		return planapi.Plan{}, errors.Errorf("machine %s/%s has no desired version set", machine.Namespace, machine.Name)
	}

	if agentConfig.AirGapped {
		return planapi.Plan{}, errors.Errorf(
			"machine %s/%s is air-gapped, which in-place update plans do not support yet", machine.Namespace, machine.Name)
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

		installEnv = append(installEnv, "INSTALL_RKE2_TYPE=server")
		installEnv = append(installEnv, "INSTALL_RKE2_EXEC=server")
	} else {
		installEnv = append(installEnv, "INSTALL_RKE2_TYPE=agent")
		installEnv = append(installEnv, "INSTALL_RKE2_EXEC=agent")
	}

	image := installerImage(agentConfig.SystemDefaultRegistry, version)

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

// installerImage builds the system-agent-installer-rke2 image reference for version, prefixed
// with systemDefaultRegistry when set (mirrors rancher/pkg/capr/planner.getInstallerImage).
func installerImage(systemDefaultRegistry, version string) string {
	image := fmt.Sprintf("%s:%s", installerImageRepository, strings.ReplaceAll(version, "+", "-"))
	if systemDefaultRegistry == "" {
		return image
	}

	return systemDefaultRegistry + "/" + image
}

// convertFiles translates RKE2ConfigSpec.Files into system-agent Plan.Files. planapi.File.Content
// is always plain base64 of the final raw bytes (system-agent writes it via a base64 decode only,
// with no notion of gzip), so each bootstrapv1.File's Encoding is resolved to raw bytes first and
// then re-encoded as base64.
//
// ContentFrom (Secret/ConfigMap-sourced content) must already be resolved into Content by the
// caller (see ExtensionHandlers.resolveDesiredFiles); a file still carrying ContentFrom here fails
// loudly rather than silently dropping its content.
func convertFiles(files []bootstrapv1.File) ([]planapi.File, error) {
	if len(files) == 0 {
		return nil, nil
	}

	converted := make([]planapi.File, 0, len(files))

	for _, f := range files {
		if f.ContentFrom != nil {
			return nil, errors.Errorf("file %s uses contentFrom, which was not resolved before plan construction", f.Path)
		}

		raw, err := decodeFileContent(f.Content, f.Encoding)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode content for file %s", f.Path)
		}

		uid, gid, err := parseOwner(f.Owner)
		if err != nil {
			return nil, errors.Wrapf(err, "file %s", f.Path)
		}

		converted = append(converted, planapi.File{
			Path:        f.Path,
			Permissions: f.Permissions,
			Content:     base64.StdEncoding.EncodeToString(raw),
			UID:         uid,
			GID:         gid,
		})
	}

	return converted, nil
}

// parseOwner translates a bootstrapv1.File's Owner ("user:group") into a UID/GID pair.
// Only the common "root:root" default (or an empty Owner) and explicit numeric "uid:gid" forms
// can be translated without resolving a user database on the target node, which isn't available
// at plan-build time; any other symbolic owner is rejected rather than silently defaulting to
// root, since DoCanUpdateMachine claims the complete Files field including Owner.
func parseOwner(owner string) (uid, gid int, err error) {
	if owner == "" || owner == "root:root" {
		return 0, 0, nil
	}

	parts := strings.SplitN(owner, ":", 2)
	if len(parts) == 2 {
		if u, uErr := strconv.Atoi(parts[0]); uErr == nil {
			if g, gErr := strconv.Atoi(parts[1]); gErr == nil {
				return u, g, nil
			}
		}
	}

	return 0, 0, errors.Errorf(
		"owner %q is not supported: only root:root or a numeric uid:gid can be translated without node-side user resolution", owner)
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
