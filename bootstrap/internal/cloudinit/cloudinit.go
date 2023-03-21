/*
Copyright 2022 SUSE.

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

//nolint:gochecknoglobals
package cloudinit

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1alpha1"
)

var defaultTemplateFuncMap = template.FuncMap{
	"Indent": templateYAMLIndent,
}

func templateYAMLIndent(i int, input string) string {
	split := strings.Split(input, "\n")
	ident := "\n" + strings.Repeat(" ", i)

	return strings.Repeat(" ", i) + strings.Join(split, ident)
}

const (
	defaultYamlIndent = 2
	cloudConfigHeader = `## template: jinja
#cloud-config
`

	filesTemplate = `{{ define "files" -}}
write_files:{{ range . }}
-   path: {{.Path}}
    {{ if ne .Encoding "" -}}
    encoding: "{{.Encoding}}"
    {{ end -}}
    {{ if ne .Owner "" -}}
    owner: {{.Owner}}
    {{ end -}}
    {{ if ne .Permissions "" -}}
    permissions: '{{.Permissions}}'
    {{ end -}}
    content: |
{{.Content | Indent 6}}
{{- end -}}
{{- end -}}
`

	commandsTemplate = `{{- define "commands" -}}
{{ range . }}
  - {{printf "%q" .}}
{{- end -}}
{{- end -}}
`
	sentinelFileCommand = `echo success > /run/cluster-api/bootstrap-success.complete`

	ntpTemplate = `{{ define "ntp" -}}{{ if . -}}
ntp:
  enabled: true
  servers:{{ range .}}
  - {{printf "%q" .}}
    {{- end -}}	
{{- end -}}
{{- end -}}
`
)

// BaseUserData is shared across all the various types of files written to disk.
type BaseUserData struct {
	Header              string
	PreRKE2Commands     []string
	DeployRKE2Commands  []string
	PostRKE2Commands    []string
	WriteFiles          []bootstrapv1.File
	ConfigFile          bootstrapv1.File
	RKE2Version         string
	SentinelFileCommand string
	AirGapped           bool
	NTPServers          []string
	CISEnabled          bool
	AdditionalCloudInit string
}

func generate(kind string, tpl string, data interface{}) ([]byte, error) {
	tm := template.New(kind).Funcs(defaultTemplateFuncMap)
	if _, err := tm.Parse(filesTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse files template")
	}

	if _, err := tm.Parse(commandsTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse commands template")
	}

	if _, err := tm.Parse(ntpTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse ntp template")
	}

	t, err := tm.Parse(tpl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s template", kind)
	}

	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return nil, errors.Wrapf(err, "failed to generate %s template", kind)
	}

	return out.Bytes(), nil
}

func cleanupAdditionalCloudInit(cloudInitData string) (string, error) {
	m := make(map[string]interface{})

	err := yaml.Unmarshal([]byte(cloudInitData), m)
	if err != nil {
		return "", err
	}

	// Remove the runcmd section
	delete(m, "runcmd")

	// Remove the write_files section
	delete(m, "write_files")

	// Remove the ntp section
	delete(m, "ntp")

	bytesBuf := bytes.Buffer{}
	encoder := yaml.NewEncoder(&bytesBuf)
	encoder.SetIndent(defaultYamlIndent)

	err = encoder.Encode(m)
	if err != nil {
		return "", err
	}

	res := bytesBuf.String()
	if res == "{}\n" {
		return "", nil
	}

	return res, nil
}
