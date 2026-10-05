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

package guest

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCloudInitRenders(t *testing.T) {
	out, err := CloudInit(Params{
		Name:              "alice-desk",
		BasePort:          30016,
		AdminPassword:     "p'w",
		SSHAuthorizedKeys: []string{"ssh-ed25519 AAAA test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "#cloud-config\n") {
		t.Fatalf("missing cloud-config header:\n%s", out)
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not valid YAML: %v\n%s", err, out)
	}
	for _, want := range []string{
		"port = 30016",
		"sunshine_name = alice-desk",
		"encoder = software",
		`global_prep_cmd = [{"do":"sh -c \"deskube-resize || true\""`,
		"encoder = software",
		"--creds 'admin' 'p''w'",
		"runuser -u desk",
		"file_apps = /home/desk/.config/sunshine/apps.json",
		`"apps": [{"name": "Desktop"`,
		"/etc/deskube/sunshine.defaults",
		"- 'ssh-ed25519 AAAA test'",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestCloudInitCaptureOptional(t *testing.T) {
	out, err := CloudInit(Params{Name: "x", BasePort: 1, AdminPassword: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "capture =") {
		t.Errorf("empty capture should leave detection to Sunshine:\n%s", out)
	}
	out, _ = CloudInit(Params{Name: "x", BasePort: 1, AdminPassword: "p", Capture: "kms"})
	if !strings.Contains(out, "capture = kms") {
		t.Errorf("explicit capture missing:\n%s", out)
	}
}

func TestCloudInitNoKeys(t *testing.T) {
	out, err := CloudInit(Params{Name: "x", BasePort: 1, AdminPassword: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "ssh_authorized_keys") {
		t.Errorf("empty key list should be omitted:\n%s", out)
	}
}
