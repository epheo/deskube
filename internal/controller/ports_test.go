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
	"testing"

	"github.com/deskube/deskube/internal/sunshine"
)

func TestAllocateNodePortBase(t *testing.T) {
	used := map[int32]bool{}
	var bases []int32
	for {
		b, err := allocateNodePortBase(used)
		if err != nil {
			break
		}
		if !nodePortBaseValid(b) {
			t.Fatalf("base %d leaves the NodePort range", b)
		}
		for _, prev := range bases {
			if b-prev < sunshine.OffsetRTSP-sunshine.OffsetHTTPS {
				t.Fatalf("bases %d and %d overlap", prev, b)
			}
		}
		bases = append(bases, b)
		used[b] = true
	}
	if len(bases) != 86 {
		t.Fatalf("expected 86 windows in the NodePort range, got %d", len(bases))
	}
	if bases[0] != 30016 {
		t.Fatalf("first base %d, want 30016", bases[0])
	}
}

func TestAllocateSkipsUsed(t *testing.T) {
	b, err := allocateNodePortBase(map[int32]bool{30016: true})
	if err != nil || b != 30048 {
		t.Fatalf("got %d, %v; want 30048", b, err)
	}
}

func TestServicePorts(t *testing.T) {
	ports := servicePorts(31989, true)
	want := map[string]int32{"https": 31984, "http": 31989, "web": 31990, "video": 31998, "control": 31999, "audio": 32000, "rtsp": 32010}
	if len(ports) != len(want) {
		t.Fatalf("got %d ports, want %d", len(ports), len(want))
	}
	for _, p := range ports {
		if want[p.Name] != p.Port || p.NodePort != p.Port || p.TargetPort.IntVal != p.Port {
			t.Errorf("port %s: %+v", p.Name, p)
		}
		udp := p.Name == "video" || p.Name == "control" || p.Name == "audio"
		if udp != (p.Protocol == "UDP") {
			t.Errorf("port %s protocol %s", p.Name, p.Protocol)
		}
	}
	for _, p := range servicePorts(47989, false) {
		if p.NodePort != 0 {
			t.Errorf("load balancer port %s should not pin a node port", p.Name)
		}
	}
}
