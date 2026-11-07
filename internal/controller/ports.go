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

// allocateNodePortBase returns the lowest free base in the NodePort range.
func allocateNodePortBase(used map[int32]bool) (int32, error) {
	for base := int32(firstNodePortBase); base+sunshine.OffsetRTSP <= nodePortMax; base += basePortStep {
		if !used[base] {
			return base, nil
		}
	}
	return 0, errors.New("no free base port left in the NodePort range")
}

// nodePortBaseValid reports whether every derived port fits the NodePort range.
func nodePortBaseValid(base int32) bool {
	return base+sunshine.OffsetHTTPS >= nodePortMin && base+sunshine.OffsetRTSP <= nodePortMax
}
