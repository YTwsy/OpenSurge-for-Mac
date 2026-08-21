package controlapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

func TestLocalConnectionRefreshClosesOnlyGatewayLocalConnections(t *testing.T) {
	server := newTestServer(t)
	server.fetchConnections = func(context.Context, config.Config) (mihomo.ConnectionsSnapshot, error) {
		return mihomo.ConnectionsSnapshot{Connections: []mihomo.Connection{
			{ID: "local-process", Metadata: map[string]any{"sourceIP": "198.18.0.1", "type": "Tun", "process": "Safari"}},
			{ID: "local-shared-source", Metadata: map[string]any{"sourceIP": "198.18.0.1", "type": "Tun"}},
			{ID: "local-loopback", Metadata: map[string]any{"sourceIP": "127.0.0.1", "type": "Mixed"}},
			{ID: "local-gateway", Metadata: map[string]any{"sourceIP": "192.168.1.20"}},
			{ID: "downstream", Metadata: map[string]any{"sourceIP": "192.168.1.151"}},
		}}, nil
	}
	var closedIDs []string
	server.closeConnections = func(_ context.Context, _ config.Config, ids []string) (int, error) {
		closedIDs = append([]string(nil), ids...)
		return len(ids), nil
	}

	response := performAuthorized(server, http.MethodPost, "/api/v1/local-routing/connections/refresh", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("refresh local connections status=%d body=%s", response.Code, response.Body.String())
	}
	if !reflect.DeepEqual(closedIDs, []string{"local-process", "local-shared-source", "local-loopback", "local-gateway"}) {
		t.Fatalf("closed IDs = %#v", closedIDs)
	}
	var payload ConnectionRefreshResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Scope != connectionRefreshScopeLocal || payload.DeviceID != "" || payload.MatchedConnections != 4 || payload.ClosedConnections != 4 {
		t.Fatalf("refresh response = %#v", payload)
	}
}

func TestDeviceConnectionRefreshUsesAppliedIPv4(t *testing.T) {
	server := newTestServer(t)
	installAppliedPolicy(t, server, device.PolicySet{
		Devices:  []device.ManagedDevice{{ID: "living-room", Name: "Living Room", MAC: "aa:bb:cc:dd:ee:37", IPv4: "192.168.1.137", Profile: "home", EgressMode: device.EgressModeInheritGlobal}},
		Profiles: []device.Profile{{ID: "home", DefaultPolicies: []string{"DIRECT"}}},
	})
	server.fetchConnections = func(context.Context, config.Config) (mihomo.ConnectionsSnapshot, error) {
		return mihomo.ConnectionsSnapshot{Connections: []mihomo.Connection{
			{ID: "one", Metadata: map[string]any{"sourceIP": "192.168.1.137"}},
			{ID: "two", Metadata: map[string]any{"sourceIP": "[::ffff:192.168.1.137]:443"}},
			{ID: "ipv6", Metadata: map[string]any{"sourceIP": "fdfe:dcba:9878::37", "inboundUser": "device:living-room"}},
			{ID: "other-ipv6", Metadata: map[string]any{"sourceIP": "fdfe:dcba:9878::38", "inboundUser": "device:other"}},
			{ID: "other-device", Metadata: map[string]any{"sourceIP": "192.168.1.138"}},
			{ID: "local", Metadata: map[string]any{"sourceIP": "127.0.0.1"}},
		}}, nil
	}
	var closedIDs []string
	server.closeConnections = func(_ context.Context, _ config.Config, ids []string) (int, error) {
		closedIDs = append([]string(nil), ids...)
		return len(ids), nil
	}

	response := performAuthorized(server, http.MethodPost, "/api/v1/devices/living-room/connections/refresh", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("refresh device connections status=%d body=%s", response.Code, response.Body.String())
	}
	if !reflect.DeepEqual(closedIDs, []string{"one", "two", "ipv6"}) {
		t.Fatalf("closed IDs = %#v", closedIDs)
	}
	var payload ConnectionRefreshResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Scope != connectionRefreshScopeDevice || payload.DeviceID != "living-room" || payload.MatchedConnections != 3 || payload.ClosedConnections != 3 {
		t.Fatalf("refresh response = %#v", payload)
	}
}

func TestDeviceConnectionRefreshRejectsUnappliedAndUpstreamRouterDevices(t *testing.T) {
	server := newTestServer(t)
	response := performAuthorized(server, http.MethodPost, "/api/v1/devices/missing/connections/refresh", nil)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "applied_device_not_found") {
		t.Fatalf("missing device status=%d body=%s", response.Code, response.Body.String())
	}

	installAppliedPolicy(t, server, device.PolicySet{
		Devices:  []device.ManagedDevice{{ID: "console", MAC: "aa:bb:cc:dd:ee:05", IPv4: "192.168.1.190", Profile: "home", GatewayTarget: device.GatewayTargetUpstreamRouter}},
		Profiles: []device.Profile{{ID: "home", DefaultPolicies: []string{"DIRECT"}}},
	})
	response = performAuthorized(server, http.MethodPost, "/api/v1/devices/console/connections/refresh", nil)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "device_connections_unmanaged") {
		t.Fatalf("upstream-router device status=%d body=%s", response.Code, response.Body.String())
	}
}

func installAppliedPolicy(t *testing.T, server *Server, policy device.PolicySet) {
	t.Helper()
	cfg, err := config.LoadRuntime(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := device.CompilePolicyBundle(policy)
	if err != nil {
		t.Fatal(err)
	}
	paths := runtime.NewPaths(cfg)
	if err := device.WritePolicyBundleSnapshot(paths.DevicePolicyApplied, bundle); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.StateFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{DevicePolicyDigest: bundle.Digest}); err != nil {
		t.Fatal(err)
	}
}
