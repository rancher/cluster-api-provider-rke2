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

package butane

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/coreos/butane/config/common"
	fcos "github.com/coreos/butane/config/fcos/v1_4"
	ignition "github.com/coreos/ignition/v2/config/v3_3"
	ignitionTypes "github.com/coreos/ignition/v2/config/v3_3/types"
	"github.com/pkg/errors"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/bootstrap/internal/cloudinit"
)

// The template contains configurations for two main sections: systemd units and storage files.
// The first section defines two systemd units: rke2-install.service and chronyd.service.
// The rke2-install.service unit is enabled and is executed only once during the boot process to run the /etc/rke2-install.sh script.
// This script installs and deploys RKE2, and performs pre and post-installation commands.
// The chronyd.service unit is enabled only if NTP servers are specified.
// The second section defines storage files for the system. It creates a file at /etc/rke2-install.sh.
// If NTP servers are specified, it creates an NTP configuration file at /etc/chrony.conf.
const (
	butaneTemplate = `
variant: fcos
version: 1.4.0
systemd:
  units:
    - name: rke2-install.service
      enabled: true
      contents: |
        [Unit]
        Description=rke2-install
        Wants=network-online.target
        After=network-online.target network.target
        ConditionPathExists=!/etc/cluster-api/bootstrap-success.complete
        [Service]
        User=root
        # To not restart the unit when it exits, as it is expected.
        Type=oneshot
        ExecStart=/etc/rke2-install.sh
        [Install]
        WantedBy=multi-user.target
    {{- if .NTPServers }}
    - name: chronyd.service
      enabled: true
    {{- end }}
storage:
  filesystems:
    - path: /opt
      device: "/dev/disk/by-partlabel/p.lxroot"
      format: btrfs
      wipe_filesystem: false
      mount_options:
       - "subvol=/@/opt"
  files:
    - path: /etc/ssh/sshd_config.d/010-rke2.conf
      mode: 0600
      overwrite: true
      contents:
        inline: |
          # Use most defaults for sshd configuration.
          Subsystem sftp internal-sftp
          ClientAliveInterval 180
          UseDNS no
          UsePAM yes
          PrintLastLog no # handled by PAM
          PrintMotd no # handled by PAM
    {{- range .WriteFiles }}
    - path: {{ .Path }}
      {{- $owner := ParseOwner .Owner }}
      {{ if $owner.User -}}
      user:
        name: {{ $owner.User }}
      {{- end }}
      {{ if $owner.Group -}}
      group:
        name: {{ $owner.Group }}
      {{- end }}
      # Owner
      {{ if ne .Permissions "" -}}
      mode: {{ .Permissions }}
      {{ end -}}
      overwrite: true
      contents:
        {{ if eq .Encoding "base64" -}}
        inline: !!binary |
        {{- else -}}
        inline: |
        {{- end }}
          {{ .Content | Indent 10 }}
    {{- end }}
    - path: /etc/rke2-install.sh
      mode: 0700
      overwrite: true
      contents:
        inline: |
          #!/bin/bash
          set -euo pipefail
          {{ range .PreRKE2Commands }}
          {{ . | Indent 10 }}
          {{- end }}

          {{ range .DeployRKE2Commands }}
          {{ . | Indent 10 }}
          {{- end }}

          {{range .PostRKE2Commands }}
          {{ . | Indent 10 }}
          {{- end }}
    {{- if .NTPServers }}
    - path: /etc/chrony.conf
      mode: 0644
      overwrite: true
      contents:
        inline: |
          # Configured by RKE2 CAPI bootstrap provider
          {{- range  .NTPServers }}
          server {{ . }}
          {{- end }}
          driftfile /var/lib/chrony/drift
          makestep 1.0 3
          rtcsync
          ntsdumpdir /var/lib/chrony
          logdir /var/log/chrony
          include /etc/chrony.d/*.conf
          sourcedir /run/chrony-dhcp
    {{- end }}
`
)

func defaultTemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"Indent":         templateYAMLIndent,
		"Split":          strings.Split,
		"Join":           strings.Join,
		"MountpointName": mountpointName,
		"ParseOwner":     parseOwner,
	}
}

func mountpointName(name string) string {
	return strings.TrimPrefix(strings.ReplaceAll(name, "/", "-"), "-")
}

func templateYAMLIndent(i int, input string) string {
	split := strings.Split(input, "\n")
	ident := "\n" + strings.Repeat(" ", i)

	return strings.Join(split, ident)
}

type owner struct {
	User  *string
	Group *string
}

func parseOwner(ownerRaw string) owner {
	if ownerRaw == "" {
		return owner{}
	}

	ownerSlice := strings.Split(ownerRaw, ":")

	parseEntity := func(entity string) *string {
		if entity == "" {
			return nil
		}

		entityTrimmed := strings.TrimSpace(entity)

		return &entityTrimmed
	}

	if len(ownerSlice) == 1 {
		return owner{
			User: parseEntity(ownerSlice[0]),
		}
	}

	return owner{
		User:  parseEntity(ownerSlice[0]),
		Group: parseEntity(ownerSlice[1]),
	}
}

func renderButane(input *cloudinit.BaseUserData) ([]byte, error) {
	t := template.Must(template.New("template").Funcs(defaultTemplateFuncMap()).Parse(butaneTemplate))

	var out bytes.Buffer
	if err := t.Execute(&out, input); err != nil {
		return nil, errors.Wrapf(err, "failed to render template")
	}

	return out.Bytes(), nil
}

// butaneToIgnition converts butane bytes to an ignition v3.3 `Config`.
func butaneToIgnition(data []byte, strict bool) (ignitionTypes.Config, error) {
	// converting butane config to ignition config (bytes)
	// NB: it could be simpler to Translate directly to a Config struct
	//     but it doesn't seems to be supported (at least without an over-complicated implementation)
	ignBytes, reports, err := fcos.ToIgn3_3Bytes(data, common.TranslateBytesOptions{})
	if err != nil {
		return ignitionTypes.Config{}, fmt.Errorf("error converting to Ignition: %w", err)
	}

	if (len(reports.Entries) > 0 && strict) || reports.IsFatal() {
		return ignitionTypes.Config{}, fmt.Errorf("error converting to Ignition: %s", reports.String())
	}
	// parse the ignition config bytes to have a Config struct
	cfg, parseReport, err := ignition.Parse(ignBytes)
	if err != nil {
		return ignitionTypes.Config{}, fmt.Errorf("error parsing resulting Ignition: %w", err)
	}

	if (len(parseReport.Entries) > 0 && strict) || parseReport.IsFatal() {
		return ignitionTypes.Config{}, fmt.Errorf("error parsing resulting Ignition: %v", parseReport.String())
	}

	reports.Merge(parseReport)

	return cfg, nil
}

// Render renders the provided user data and additional butane config into an Ignition config.
func Render(input *cloudinit.BaseUserData, butaneCfg *bootstrapv1.AdditionalUserData) ([]byte, error) {
	if input == nil {
		return nil, errors.New("empty base user data")
	}

	butaneBytes, err := renderButane(input)
	if err != nil {
		return nil, err
	}
	// the base config is derived from the static template above, so treat it as strict
	cfg, err := butaneToIgnition(butaneBytes, true)
	if err != nil {
		return nil, errors.Wrap(err, "converting base config to Ignition")
	}

	if butaneCfg != nil && butaneCfg.Config != "" {
		addCfg, err := butaneToIgnition([]byte(butaneCfg.Config), butaneCfg.Strict)
		if err != nil {
			return nil, errors.Wrap(err, "converting additional config to Ignition")
		}

		if strings.Contains(butaneCfg.Config, "variant: flatcar") {
			// this is flatcar drop /opt filesystem from default config
			cfg.Storage.Filesystems = []ignitionTypes.Filesystem{}
		}

		cfg = ignition.Merge(cfg, addCfg)
	}

	userData, err := json.Marshal(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "marshaling Ignition config into JSON")
	}

	return userData, nil
}

// EncapsulateGzippedConfig takes a gzipped Ignition config and encapsulates it in an Ignition config with compression.
func EncapsulateGzippedConfig(gzippedConfig []byte) ([]byte, error) {
	comp := "gzip"
	dataSource := "data:text/plain;base64," + base64.StdEncoding.EncodeToString(gzippedConfig)

	encapCfg := ignitionTypes.Config{
		Ignition: ignitionTypes.Ignition{
			Version: "3.3.0",
			Config: ignitionTypes.IgnitionConfig{
				Replace: ignitionTypes.Resource{
					Compression: &comp,
					Source:      &dataSource,
				},
			},
		},
	}

	cfg, err := json.Marshal(encapCfg)
	if err != nil {
		return nil, errors.Wrap(err, "marshaling Ignition config into JSON")
	}

	return cfg, nil
}
