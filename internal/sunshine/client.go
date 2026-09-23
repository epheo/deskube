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

// Package sunshine talks to the Sunshine host inside a desktop:
// the GameStream serverinfo endpoint and the admin web API.
package sunshine

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Port offsets from the base port, shared by Sunshine and Moonlight.
// RTSP advertises the UDP ports, so the guest and the outside must agree.
const (
	OffsetHTTPS   int32 = -5
	OffsetHTTP    int32 = 0
	OffsetWeb     int32 = 1
	OffsetVideo   int32 = 9
	OffsetControl int32 = 10
	OffsetAudio   int32 = 11
	OffsetRTSP    int32 = 21
)

// Port is one of the seven ports a desktop needs.
type Port struct {
	Name     string
	Offset   int32
	Protocol string
}

// Ports lists every port in the order Moonlight uses them.
var Ports = []Port{
	{"https", OffsetHTTPS, "TCP"},
	{"http", OffsetHTTP, "TCP"},
	{"web", OffsetWeb, "TCP"},
	{"video", OffsetVideo, "UDP"},
	{"control", OffsetControl, "UDP"},
	{"audio", OffsetAudio, "UDP"},
	{"rtsp", OffsetRTSP, "TCP"},
}

// ServerInfo is the subset of /serverinfo the operator acts on.
type ServerInfo struct {
	StatusCode int    `xml:"status_code,attr"`
	Hostname   string `xml:"hostname"`
	HTTPSPort  int32  `xml:"HttpsPort"`
	PairStatus int    `xml:"PairStatus"`
	State      string `xml:"state"`
}

// SessionState maps the GameStream state to free or busy.
func (s *ServerInfo) SessionState() string {
	switch s.State {
	case "SUNSHINE_SERVER_BUSY":
		return "busy"
	case "SUNSHINE_SERVER_FREE":
		return "free"
	default:
		return s.State
	}
}

var plainClient = &http.Client{Timeout: 5 * time.Second}

// GetServerInfo probes the unauthenticated HTTP port.
func GetServerInfo(ctx context.Context, host string, basePort int32) (*ServerInfo, error) {
	u := "http://" + net.JoinHostPort(host, strconv.Itoa(int(basePort+OffsetHTTP))) + "/serverinfo"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := plainClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, err
	}
	var info ServerInfo
	if err := xml.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("serverinfo from %s: %w", u, err)
	}
	return &info, nil
}

// Client calls the admin web API with basic auth.
// The certificate is self signed, so verification is off.
type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

// NewClient targets the web port derived from basePort.
func NewClient(host string, basePort int32, username, password string) *Client {
	return &Client{
		baseURL:  "https://" + net.JoinHostPort(host, strconv.Itoa(int(basePort+OffsetWeb))),
		username: username,
		password: password,
		http: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
			},
		},
	}
}

// PendingPairing is a Moonlight client waiting for its PIN.
type PendingPairing struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: %s", method, path, resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// PendingPairings lists clients that started pairing and wait for a PIN.
func (c *Client) PendingPairings(ctx context.Context) ([]PendingPairing, error) {
	var out struct {
		Pairings []PendingPairing `json:"pairings"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/pin", nil, &out); err != nil {
		return nil, err
	}
	return out.Pairings, nil
}

// ApprovePairing submits the PIN for one pending pairing.
// A wrong PIN comes back as status false, not as an HTTP error.
func (c *Client) ApprovePairing(ctx context.Context, id, pin, name string) (bool, error) {
	in := map[string]string{"pairing_id": id, "pin": pin, "name": name}
	var out struct {
		Status bool `json:"status"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/pin", in, &out); err != nil {
		return false, err
	}
	return out.Status, nil
}
