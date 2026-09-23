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

package sunshine

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// Real serverinfo from Sunshine 2026.914 on the proof VM, trimmed.
const serverInfoXML = `<?xml version="1.0" encoding="utf-8"?>
<root status_code="200"><hostname>desk-proof</hostname><appversion>7.1.431.-1</appversion><GfeVersion>3.23.0.74</GfeVersion><uniqueid>31B90309</uniqueid><HttpsPort>31984</HttpsPort><ExternalPort>31989</ExternalPort><PairStatus>0</PairStatus><state>SUNSHINE_SERVER_FREE</state></root>`

func TestServerInfoParse(t *testing.T) {
	var info ServerInfo
	if err := xml.Unmarshal([]byte(serverInfoXML), &info); err != nil {
		t.Fatal(err)
	}
	if info.StatusCode != 200 || info.Hostname != "desk-proof" || info.HTTPSPort != 31984 {
		t.Fatalf("%+v", info)
	}
	if info.SessionState() != "free" {
		t.Fatalf("state %q", info.SessionState())
	}
	info.State = "SUNSHINE_SERVER_BUSY"
	if info.SessionState() != "busy" {
		t.Fatalf("state %q", info.SessionState())
	}
}

func TestGetServerInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/serverinfo" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(serverInfoXML))
	}))
	defer srv.Close()
	host, port := splitHostPort(t, srv.URL)
	info, err := GetServerInfo(context.Background(), host, port)
	if err != nil {
		t.Fatal(err)
	}
	if info.Hostname != "desk-proof" {
		t.Fatalf("%+v", info)
	}
}

func TestPairingAPI(t *testing.T) {
	var approved map[string]string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/pin":
			_, _ = w.Write([]byte(`{"pairings":[{"address":"10.0.0.1","id":"abc","name":"laptop"}]}`))
		case "POST /api/pin":
			_ = json.NewDecoder(r.Body).Decode(&approved)
			_, _ = w.Write([]byte(`{"status":` + strconv.FormatBool(approved["pin"] == "4693") + `}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	host, webPort := splitHostPort(t, srv.URL)
	c := NewClient(host, webPort-OffsetWeb, "admin", "secret")

	pending, err := c.PendingPairings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != "abc" || pending[0].Address != "10.0.0.1" {
		t.Fatalf("%+v", pending)
	}
	ok, err := c.ApprovePairing(context.Background(), "abc", "0000", "laptop")
	if err != nil || ok {
		t.Fatalf("wrong PIN accepted: %v %v", ok, err)
	}
	ok, err = c.ApprovePairing(context.Background(), "abc", "4693", "laptop")
	if err != nil || !ok {
		t.Fatalf("right PIN rejected: %v %v", ok, err)
	}
	if approved["pairing_id"] != "abc" || approved["name"] != "laptop" {
		t.Fatalf("%+v", approved)
	}

	bad := NewClient(host, webPort-OffsetWeb, "admin", "wrong")
	if _, err := bad.PendingPairings(context.Background()); err == nil {
		t.Fatal("bad credentials should fail")
	}
}

func splitHostPort(t *testing.T, url string) (string, int32) {
	t.Helper()
	hp := url[len("http://"):]
	if len(url) > 8 && url[:8] == "https://" {
		hp = url[len("https://"):]
	}
	host, portStr, err := net.SplitHostPort(hp)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)
	return host, int32(port)
}
