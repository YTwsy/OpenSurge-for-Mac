package controlapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/macosnetwork"
	"open-mihomo-gateway/internal/mihomo"
)

func TestDeviceNeighborHistoryDistinguishesRotationFromFlapping(t *testing.T) {
	var h deviceNeighborHistory
	now := time.Now()
	sample := func(scope, mac string, at time.Time) ObservedDevice {
		rows := []ObservedDevice{{IP: "192.168.1.10", MAC: mac, ActiveConnections: 1, NeighborObserved: true}}
		h.annotate(scope, rows, at)
		return rows[0]
	}
	if sample("en0", "aa:bb:cc:dd:ee:01", now).MACConflict {
		t.Fatal("initial sample conflicts")
	}
	if sample("en0", "ee:f7:1b:ec:bc:a5", now.Add(time.Second)).MACConflict {
		t.Fatal("single rotation conflicts")
	}
	if sample("en0", "ee:f7:1b:ec:bc:a5", now.Add(2*time.Second)).MACConflict {
		t.Fatal("repeated samples count as changes")
	}
	if !sample("en0", "aa:bb:cc:dd:ee:01", now.Add(3*time.Second)).MACConflict {
		t.Fatal("flapping was not detected")
	}
	if sample("en0", "aa:bb:cc:dd:ee:01", now.Add(3*time.Minute)).MACConflict {
		t.Fatal("expired conflict persists")
	}
	if sample("en7", "ee:f7:1b:ec:bc:a5", now.Add(3*time.Minute)).MACConflict {
		t.Fatal("another LAN inherits history")
	}
}

func TestDeviceRebindRejectsAmbiguousOrChangedEvidence(t *testing.T) {
	request := deviceRebindRequest{DeviceID: "phone", FromIPv4: "192.168.1.10", ToIPv4: "192.168.1.10", FromMAC: "aa:bb:cc:dd:ee:01", ToMAC: "ee:f7:1b:ec:bc:a5"}
	candidate := ObservedDevice{IP: request.ToIPv4, MAC: request.ToMAC, ActiveConnections: 1, NeighborObserved: true}
	for _, test := range []struct {
		name      string
		rows      []ObservedDevice
		owners    []device.ManagedDevice
		wantError bool
	}{
		{name: "single rotation", rows: []ObservedDevice{candidate}},
		{name: "occupied MAC", rows: []ObservedDevice{candidate}, owners: []device.ManagedDevice{{ID: "other", MAC: request.ToMAC}}, wantError: true},
		{name: "occupied IP", rows: []ObservedDevice{candidate}, owners: []device.ManagedDevice{{ID: "other", IPv4: request.ToIPv4}}, wantError: true},
		{name: "missing", wantError: true},
		{name: "no traffic", rows: []ObservedDevice{{IP: candidate.IP, MAC: candidate.MAC, NeighborObserved: true}}, wantError: true},
		{name: "changed MAC", rows: []ObservedDevice{{IP: candidate.IP, MAC: "aa:bb:cc:dd:ee:03", ActiveConnections: 1, NeighborObserved: true}}, wantError: true},
		{name: "flapping", rows: []ObservedDevice{{IP: candidate.IP, MAC: candidate.MAC, ActiveConnections: 1, NeighborObserved: true, MACConflict: true}}, wantError: true},
		{name: "original still active", rows: []ObservedDevice{candidate, {IP: "192.168.1.11", MAC: request.FromMAC, ActiveConnections: 1, NeighborObserved: true}}, wantError: true},
		{name: "multiple IPs", rows: []ObservedDevice{candidate, {IP: "192.168.1.11", MAC: request.ToMAC, ActiveConnections: 1, NeighborObserved: true}}, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateDeviceRebind(request, test.owners, test.rows); (err != nil) != test.wantError {
				t.Fatalf("error=%v", err)
			}
		})
	}
	request.ToMAC = request.FromMAC
	request.ToIPv4 = "192.168.1.11"
	candidate.MAC = request.ToMAC
	candidate.IP = request.ToIPv4
	if err := validateDeviceRebind(request, nil, []ObservedDevice{candidate}); err != nil {
		t.Fatal(err)
	}
	if err := validateDeviceRebind(request, nil, []ObservedDevice{candidate, {IP: request.FromIPv4, ActiveConnections: 1}}); err == nil {
		t.Fatal("old IP is still active")
	}
}

type identityConfigurationRunner struct {
	fakeConfigurationRunner
	payload []byte
}

func (r *identityConfigurationRunner) ApplyDevicePolicy(ctx context.Context, path, revision string, payload []byte) (string, error) {
	r.payload = append([]byte(nil), payload...)
	return r.fakeConfigurationRunner.ApplyDevicePolicy(ctx, path, revision, payload)
}

func TestDeviceRebindEndpointPreservesPolicyAndRechecksEvidence(t *testing.T) {
	for _, mode := range []string{"mac", "ip", "stale", "unavailable", "revision", "owner"} {
		t.Run(mode, func(t *testing.T) {
			server := newTestServer(t)
			cfg, err := config.LoadRuntime(server.configPath)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Gateway.Mode = config.GatewayModeSameLAN
			cfg.DHCP.Enabled = false
			if err := os.WriteFile(server.configPath, []byte(config.Render(cfg)), 0600); err != nil {
				t.Fatal(err)
			}
			policy := device.PolicySet{Devices: []device.ManagedDevice{{ID: "phone", Name: "My phone", MAC: "aa:bb:cc:dd:ee:01", IPv4: "192.168.1.110", Profile: "phone-policy", EgressMode: device.EgressModeDedicated}}, Profiles: []device.Profile{{ID: "phone-policy", DefaultPolicies: []string{"DIRECT", "REJECT"}, Rules: []device.Rule{{ID: "local", Action: "DIRECT", Match: device.RuleMatch{Domains: []string{"example.org"}}}}}}}
			req := deviceRebindRequest{DeviceID: "phone", FromIPv4: policy.Devices[0].IPv4, FromMAC: policy.Devices[0].MAC, ToIPv4: policy.Devices[0].IPv4, ToMAC: "ee:f7:1b:ec:bc:a5"}
			if mode == "ip" {
				req.ToMAC = req.FromMAC
				req.ToIPv4 = "192.168.1.111"
			}
			if mode == "owner" {
				policy.Devices = append(policy.Devices, device.ManagedDevice{ID: "other", IPv4: "192.168.1.112", MAC: req.ToMAC, Profile: "phone-policy", EgressMode: device.EgressModeDedicated})
			}
			raw, _ := json.Marshal(policy)
			if err := os.WriteFile(cfg.DevicePolicy.File, raw, 0600); err != nil {
				t.Fatal(err)
			}
			original, err := device.LoadPolicyBundle(cfg.DevicePolicy.File)
			if err != nil {
				t.Fatal(err)
			}
			runner := &identityConfigurationRunner{}
			server.configRunner = runner
			server.fetchConnections = func(context.Context, config.Config) (mihomo.ConnectionsSnapshot, error) {
				return mihomo.ConnectionsSnapshot{Connections: []mihomo.Connection{{Metadata: map[string]any{"sourceIP": req.ToIPv4}}}}, nil
			}
			server.discoverNeighbors = func(context.Context, string) ([]macosnetwork.Neighbor, error) {
				if mode == "unavailable" {
					return nil, errors.New("unavailable")
				}
				mac := req.ToMAC
				if mode == "stale" {
					mac = "aa:bb:cc:dd:ee:03"
				}
				return []macosnetwork.Neighbor{{IP: req.ToIPv4, MAC: mac, Interface: "en0"}}, nil
			}
			body, _ := json.Marshal(req)
			request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:61767/api/v1/device-policy/rebind", bytes.NewReader(body))
			request.Header.Set("Authorization", "Bearer "+server.token)
			request.Header.Set("If-Match", original.Digest)
			if mode == "revision" {
				request.Header.Set("If-Match", "old")
			}
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if mode != "mac" && mode != "ip" {
				if response.Code != http.StatusConflict || runner.payload != nil {
					t.Fatalf("unsafe save: %d %s", response.Code, response.Body.String())
				}
				return
			}
			if response.Code != http.StatusOK {
				t.Fatalf("%d %s", response.Code, response.Body.String())
			}
			var saved device.PolicySet
			if err := json.Unmarshal(runner.payload, &saved); err != nil {
				t.Fatal(err)
			}
			expected := original.Policy
			expected.Devices[0].MAC = req.ToMAC
			expected.Devices[0].IPv4 = req.ToIPv4
			if !reflect.DeepEqual(saved, expected) {
				t.Fatalf("policy changed beyond address fields: %#v", saved)
			}
			before, _ := device.CompilePolicyBundle(policy)
			after, _ := device.CompilePolicyBundle(saved)
			if !reflect.DeepEqual(before.Compiled.SelectorGroups, after.Compiled.SelectorGroups) {
				t.Fatal("selector namespace or options changed")
			}
			if !strings.Contains(response.Body.String(), req.ToMAC) {
				t.Fatal("updated address missing")
			}
		})
	}
}
