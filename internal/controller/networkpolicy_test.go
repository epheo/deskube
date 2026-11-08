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

package controller

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Desktops admit the manager by pod label and namespace. envtest
// enforces no policy, so a manager these miss would only show on a
// cluster, as pairing that never reaches Sunshine.
func TestManagerPodMatchesDesktopPolicies(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "config", "manager", "manager.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	decoder := yaml.NewYAMLOrJSONDecoder(f, 4096)
	found := false
	for {
		dep := &appsv1.Deployment{}
		err := decoder.Decode(dep)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if dep.Kind != "Deployment" {
			continue
		}
		found = true
		if got := dep.Spec.Template.Labels[managerPodLabel]; got != managerPodValue {
			t.Errorf("manager pod label %s=%q, want %q", managerPodLabel, got, managerPodValue)
		}
		told := false
		for _, c := range dep.Spec.Template.Spec.Containers {
			for _, e := range c.Env {
				ref := e.ValueFrom
				if e.Name == "POD_NAMESPACE" && ref != nil && ref.FieldRef != nil && ref.FieldRef.FieldPath == "metadata.namespace" {
					told = true
				}
			}
		}
		if !told {
			t.Error("the manager is not told its namespace in POD_NAMESPACE")
		}
	}
	if !found {
		t.Fatal("no Deployment in manager.yaml")
	}
}
