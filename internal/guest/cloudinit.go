/*
Copyright 2025 Thibaut Lapierre.

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

// Package guest renders what the operator injects into a desktop at boot.
package guest

import (
	"bytes"
	"strings"
	"text/template"
)

// prepCmd follows the client resolution and never fails the session:
// a non zero prep command makes Sunshine abort the launch.
const prepCmd = `[{"do":"sh -c \"/usr/local/bin/deskube-resize || true\"",` +
	`"undo":"sh -c \"/usr/local/bin/deskube-resize reset || true\""}]`

// Params is everything that differs between two desktops from one image.
type Params struct {
	Name              string
	BasePort          int32
	AdminUser         string
	AdminPassword     string
	SSHAuthorizedKeys []string
	Encoder           string
	Capture           string
	// PrepCmd is filled in by CloudInit.
	PrepCmd string
}

// The image already carries Xorg, XFCE, Sunshine, the user units, and
// deskube-resize, which follows the client resolution at stream start.
// Only identity, port, and credentials are set here, then Sunshine restarts.
var cloudInit = template.Must(template.New("cloud-init").Funcs(template.FuncMap{"q": yamlQuote}).Parse(
	`#cloud-config
hostname: {{ q .Name }}
users:
  - name: desk
    groups: [wheel, input]
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    lock_passwd: true
{{- if .SSHAuthorizedKeys }}
    ssh_authorized_keys:
{{- range .SSHAuthorizedKeys }}
      - {{ q . }}
{{- end }}
{{- end }}
write_files:
  - path: /home/desk/.config/sunshine/sunshine.conf
    owner: desk:desk
    defer: true
    content: |
      sunshine_name = {{ .Name }}
      port = {{ .BasePort }}
      address_family = ipv4
      origin_web_ui_allowed = wan
      capture = {{ .Capture }}
      encoder = {{ .Encoder }}
      sw_preset = superfast
      sw_tune = zerolatency
      global_prep_cmd = {{ .PrepCmd }}
      min_log_level = info
      file_state = /home/desk/.config/sunshine/sunshine_state.json
      credentials_file = /home/desk/.config/sunshine/sunshine_state.json
      log_path = /home/desk/.config/sunshine/sunshine.log
runcmd:
  - chown -R desk:desk /home/desk/.config
  - sudo -u desk /usr/bin/sunshine /home/desk/.config/sunshine/sunshine.conf
    --creds {{ q .AdminUser }} {{ q .AdminPassword }}
  - systemctl --user -M desk@ daemon-reload
  - systemctl --user -M desk@ restart sunshine.service
  - touch /var/lib/cloud/deskube-provisioned
`))

// CloudInit renders the NoCloud user data for one desktop.
func CloudInit(p Params) (string, error) {
	if p.AdminUser == "" {
		p.AdminUser = "admin"
	}
	if p.Encoder == "" {
		p.Encoder = "software"
	}
	if p.Capture == "" {
		p.Capture = "x11"
	}
	p.PrepCmd = prepCmd
	var buf bytes.Buffer
	if err := cloudInit.Execute(&buf, p); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// yamlQuote single quotes a scalar so keys and passwords survive YAML
// and the shell line in runcmd alike.
func yamlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
