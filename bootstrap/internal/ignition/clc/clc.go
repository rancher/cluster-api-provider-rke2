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

package clc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	clct "github.com/coreos/ignition/v2/config/v3_3"
	ignition "github.com/coreos/ignition/v2/config/v3_3"
	ignitionTypes "github.com/coreos/ignition/v2/config/v3_3/types"
	"github.com/pkg/errors"

	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1alpha1"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/internal/cloudinit"
)

// The template contains configurations for two main sections: systemd units and storage files.
// The first section defines two systemd units: rke2-install.service and ntpd.service.
// The rke2-install.service unit is enabled and is executed only once during the boot process to run the /etc/rke2-install.sh script.
// This script installs and deploys RKE2, and performs pre and post-installation commands.
// The ntpd.service unit is enabled only if NTP servers are specified.
// The second section defines storage files for the system. It creates a file at /etc/rke2-install.sh. If CISEnabled is set to true,
// it runs an additional CIS script to enforce system security standards. If NTP servers are specified,
// it creates an NTP configuration file at /etc/ntp.conf.
const (
	clcTemplate = `---
systemd:
  units:
    - name: rke2-install.service
      enabled: true
      contents: |
        [Unit]
        Description=rke2-install
        [Service]
        # To not restart the unit when it exits, as it is expected.
        Type=oneshot
        ExecStart=/etc/rke2-install.sh
        [Install]
        WantedBy=multi-user.target
    {{- if .NTPServers }}
    - name: ntpd.service
      enabled: true
    {{- end }}
storage:
  files:
    - path: /etc/ssh/sshd_config
      mode: 0600
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
      contents:
        inline: |
          #!/bin/bash
          set -e
          {{ range .PreRKE2Commands }}
          {{ . | Indent 10 }}
          {{- end }}

		  {{- if .CISEnabled }}
  		  /opt/rke2-cis-script.sh
		  {{ end }}

          {{ range .DeployRKE2Commands }}
          {{ . | Indent 10 }}
          {{- end }}

          mkdir -p /run/cluster-api && echo success > /run/cluster-api/bootstrap-success.complete
          {{range .PostRKE2Commands }}
          {{ . | Indent 10 }}
          {{- end }}
    {{- if .NTPServers }}
    - path: /etc/ntp.conf
      mode: 0644
      contents:
        inline: |
          # Common pool
          {{- range  .NTPServers }}
          server {{ . }}
          {{- end }}

          # Warning: Using default NTP settings will leave your NTP
          # server accessible to all hosts on the Internet.

          # If you want to deny all machines (including your own)
          # from accessing the NTP server, uncomment:
          #restrict default ignore

          # Default configuration:
          # - Allow only time queries, at a limited rate, sending KoD when in excess.
          # - Allow all local queries (IPv4, IPv6)
          restrict default nomodify nopeer noquery notrap limited kod
          restrict 127.0.0.1
          restrict [::1]
    {{- end }}
`
)

func defaultTemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"Indent":     templateYAMLIndent,
		"ParseOwner": parseOwner,
	}
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

func renderCLC(input *cloudinit.BaseUserData) ([]byte, error) {
	t := template.Must(template.New("template").Funcs(defaultTemplateFuncMap()).Parse(clcTemplate))

	var out bytes.Buffer
	if err := t.Execute(&out, input); err != nil {
		return nil, errors.Wrapf(err, "failed to render template")
	}

	return out.Bytes(), nil
}

// Render renders the provided user data and CLC snippets into Ignition config.
func Render(input *cloudinit.BaseUserData, additionalConfig *bootstrapv1.AdditionalUserData) ([]byte, error) {
	if input == nil {
		return nil, errors.New("empty base user data")
	}

	clcBytes, err := renderCLC(input)
	if err != nil {
		return nil, errors.Wrapf(err, "rendering CLC configuration")
	}

	userData, _, err := buildIgnitionConfig(clcBytes, additionalConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "building Ignition config")
	}

	return userData, nil
}

func buildIgnitionConfig(baseCLC []byte, additionalConfig *bootstrapv1.AdditionalUserData) ([]byte, string, error) {
	// We control baseCLC config, so treat it as strict.
	ign, _, err := clcToIgnition(baseCLC, true)
	if err != nil {
		return nil, "", errors.Wrapf(err, "converting generated CLC to Ignition")
	}

	var clcWarnings string

	if additionalConfig != nil && additionalConfig.Config != "" {
		additionalIgn, warnings, err := clcToIgnition([]byte(additionalConfig.Config), additionalConfig.Strict)
		if err != nil {
			return nil, "", errors.Wrapf(err, "converting additional CLC to Ignition")
		}

		clcWarnings = warnings

		ign = ignition.Merge(ign, additionalIgn)
	}

	userData, err := json.Marshal(&ign)
	if err != nil {
		return nil, "", errors.Wrapf(err, "marshaling generated Ignition config into JSON")
	}

	return userData, clcWarnings, nil
}

func clcToIgnition(data []byte, strict bool) (ignitionTypes.Config, string, error) {
	clc, reports, err := clct.Parse(data)

	if (len(reports.Entries) > 0 && strict) || reports.IsFatal() {
		return ignitionTypes.Config{}, "", fmt.Errorf("error parsing Container Linux Config: %v", reports.String())
	}

	return clc, reports.String(), err
}
