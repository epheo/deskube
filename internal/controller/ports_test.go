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

	corev1 "k8s.io/api/core/v1"

	vdiv1alpha1 "github.com/epheo/deskube/api/v1alpha1"
	"github.com/epheo/deskube/internal/sunshine"
)

func TestAllocateNodePortBase(t *testing.T) {
	held := heldPorts{}
	var bases []int32
	for {
		b, err := allocateNodePortBase(held)
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
		held.holdWindow(b, "VdiInstance ns/d")
	}
	if len(bases) != 86 {
		t.Fatalf("expected 86 windows in the NodePort range, got %d", len(bases))
	}
	if bases[0] != 30016 {
		t.Fatalf("first base %d, want 30016", bases[0])
	}
}

func TestAllocateSkipsHeldPorts(t *testing.T) {
	for _, tc := range []struct {
		held int32
		want int32
	}{
		{30011, 30048}, // https of the first window
		{30017, 30048}, // web, held even when the desktop does not publish it
		{30037, 30048}, // rtsp
		{30020, 30016}, // between the window's ports, not one of them
		{30043, 30016}, // the next window's https
	} {
		b, err := allocateNodePortBase(heldPorts{tc.held: "Service ns/s"})
		if err != nil || b != tc.want {
			t.Errorf("port %d held: got %d, %v; want %d", tc.held, b, err, tc.want)
		}
	}
}

func TestOffGridWindowConflict(t *testing.T) {
	held := heldPorts{}
	held.holdWindow(30016, "VdiInstance ns/alice")
	held.hold(30026, "Service ns/late")
	// Base 30026 is off the grid; its http port is alice's control port.
	port, holder, taken := held.conflict(30026)
	if !taken || port != 30026 || holder != "VdiInstance ns/alice" {
		t.Fatalf("got %d %q %v", port, holder, taken)
	}
	if _, _, taken := held.conflict(30048); taken {
		t.Fatal("30048 shares no port with 30016")
	}
}

func TestSunshineHost(t *testing.T) {
	vdi := &vdiv1alpha1.VdiInstance{Status: vdiv1alpha1.VdiInstanceStatus{Address: "203.0.113.10:30016"}}
	pod := &corev1.Pod{Status: corev1.PodStatus{PodIP: "10.128.0.7"}}
	inside := &VdiInstanceReconciler{}
	if h, err := inside.sunshineHost(vdi, pod); err != nil || h != "10.128.0.7" {
		t.Errorf("inside: got %q, %v", h, err)
	}
	for _, p := range []*corev1.Pod{nil, {}} {
		if h, err := inside.sunshineHost(vdi, p); err == nil {
			t.Errorf("inside, pod %v: got %q, want an error", p, h)
		}
	}
	outside := &VdiInstanceReconciler{ProbeExternal: true}
	if h, err := outside.sunshineHost(vdi, pod); err != nil || h != "203.0.113.10" {
		t.Errorf("outside: got %q, %v", h, err)
	}
	if h, err := outside.sunshineHost(&vdiv1alpha1.VdiInstance{}, pod); err == nil {
		t.Errorf("outside, no address: got %q, want an error", h)
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
