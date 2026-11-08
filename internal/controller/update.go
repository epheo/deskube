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
	"net/http"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"

	"github.com/epheo/deskube/internal/sunshine"
)

const (
	sessionBusy = "busy"
	sessionFree = "free"

	reasonSessionBusy    = "SessionBusy"
	reasonSessionUnknown = "SessionUnknown"
)

// heldUpdate is a pod change kept back while a session may run.
type heldUpdate struct {
	reason  string
	changes []string
}

// sessionRead is one answer from Sunshine's /serverinfo.
type sessionRead struct {
	info *sunshine.ServerInfo
	err  error
}

func (s *sessionRead) answered() bool {
	return s.err == nil && s.info.StatusCode == http.StatusOK
}

// templateChanges names what differs between the running pod template
// and the wanted one, the desktop container's fields by their own name.
// Semantic equality, as CreateOrUpdate's: a field the API server
// defaulted and the operator left alone is no change.
func templateChanges(running, wanted *corev1.PodTemplateSpec) []string {
	var names []string
	if !equality.Semantic.DeepEqual(running.ObjectMeta, wanted.ObjectMeta) {
		names = append(names, "metadata")
	}
	runningPod, wantedPod := running.Spec, wanted.Spec
	if len(runningPod.Containers) == 1 && len(wantedPod.Containers) == 1 {
		names = append(names, fieldChanges(runningPod.Containers[0], wantedPod.Containers[0])...)
		runningPod.Containers, wantedPod.Containers = nil, nil
	}
	return append(names, fieldChanges(runningPod, wantedPod)...)
}

// fieldChanges lists by JSON name the fields of a and b that differ.
func fieldChanges[T any](a, b T) []string {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	var names []string
	for i := range va.NumField() {
		if !equality.Semantic.DeepEqual(va.Field(i).Interface(), vb.Field(i).Interface()) {
			name, _, _ := strings.Cut(va.Type().Field(i).Tag.Get("json"), ",")
			names = append(names, name)
		}
	}
	return names
}
