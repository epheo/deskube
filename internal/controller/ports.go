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
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/epheo/deskube/internal/sunshine"
)

const (
	nodePortMin = 30000
	nodePortMax = 32767
	// A desktop spans base-5 to base+21. Bases 32 apart never overlap.
	basePortStep = 32
	// The lowest base whose HTTPS port still sits inside the NodePort range.
	firstNodePortBase = nodePortMin + 16
	// Moonlight's default, used when every desktop gets its own IP.
	loadBalancerBasePort = 47989
)

// heldPorts maps a node port to what holds it, as "Kind namespace/name".
type heldPorts map[int32]string

// hold keeps the first holder: a desktop is marked before its Service,
// and its name says more.
func (h heldPorts) hold(port int32, holder string) {
	if _, ok := h[port]; !ok {
		h[port] = holder
	}
}

func (h heldPorts) holdWindow(base int32, holder string) {
	for _, p := range sunshine.Ports {
		h.hold(base+p.Offset, holder)
	}
}

// conflict returns the first held port of the window on base. The web
// port counts even while unpublished, so turning the web UI on later
// cannot collide.
func (h heldPorts) conflict(base int32) (port int32, holder string, found bool) {
	for _, p := range sunshine.Ports {
		if holder, ok := h[base+p.Offset]; ok {
			return base + p.Offset, holder, true
		}
	}
	return 0, "", false
}

// allocateNodePortBase returns the lowest base whose window is free.
func allocateNodePortBase(held heldPorts) (int32, error) {
	for base := int32(firstNodePortBase); base+sunshine.OffsetRTSP <= nodePortMax; base += basePortStep {
		if _, _, taken := held.conflict(base); !taken {
			return base, nil
		}
	}
	return 0, errors.New("no free base port left in the NodePort range")
}

// portConflictError names what holds a port a pinned base needs.
type portConflictError struct {
	base, port int32
	holder     string
}

func (e *portConflictError) Error() string {
	return fmt.Sprintf("basePort %d needs node port %d, held by %s", e.base, e.port, e.holder)
}

// nodePortTaken reports whether the API server refused a Service because
// another one holds one of its node ports.
func nodePortTaken(err error) bool {
	var status apierrors.APIStatus
	if !apierrors.IsInvalid(err) || !errors.As(err, &status) || status.Status().Details == nil {
		return false
	}
	for _, cause := range status.Status().Details.Causes {
		if strings.HasSuffix(cause.Field, ".nodePort") && strings.Contains(cause.Message, "already allocated") {
			return true
		}
	}
	return false
}

// nodePortBaseValid reports whether every derived port fits the NodePort range.
func nodePortBaseValid(base int32) bool {
	return base+sunshine.OffsetHTTPS >= nodePortMin && base+sunshine.OffsetRTSP <= nodePortMax
}
