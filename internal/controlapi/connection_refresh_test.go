package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
			{ID: "local-tcp", Metadata: map[string]any{"sourceIP": "198.18.0.1", "type": "Tun", "inboundName": mihomo.SystemTUNListenerName}},
			{ID: "local-udp", Metadata: map[string]any{"sourceIP": "198.18.0.1", "type": "Tun", "inboundName": mihomo.SystemTUNListenerName}},
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
	if !reflect.DeepEqual(closedIDs, []string{"local-tcp", "local-udp", "local-loopback", "local-gateway"}) {
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

func TestPolicyConnectionRefreshMatchesExactChainAcrossMacAndDevices(t *testing.T) {
	server := newTestServer(t)
	const group = "共享/香港 策略"
	server.fetchProxyGroups = func(context.Context, config.Config) ([]mihomo.ProxyGroup, error) {
		return []mihomo.ProxyGroup{{Name: group, Type: "Selector", Selected: "new-node"}}, nil
	}
	snapshot := mihomo.ConnectionsSnapshot{Connections: []mihomo.Connection{
		{ID: "mac", Chains: []string{"old-node", group, "open-surge/mac-global"}, Metadata: map[string]any{"sourceIP": "127.0.0.1"}},
		{ID: "inherit", Chains: []string{"old-node", group}, Metadata: map[string]any{"sourceIP": "192.168.1.137"}},
		{ID: "nested-device", Chains: []string{"old-node", group, "Regional", "device/tv/default"}},
		{ID: "ipv6", Chains: []string{"old-node", group}, Metadata: map[string]any{"sourceIP": "fdfe:dcba:9878::37", "inboundUser": "device:phone"}},
		{ID: "already-new", Chains: []string{"new-node", group}},
		{ID: "same-leaf-other-group", Chains: []string{"old-node", "Streaming"}},
		{ID: "direct", Chains: []string{"DIRECT"}},
		{ID: "prefix", Chains: []string{"old-node", group + "-backup"}},
		{ID: "no-chain", Metadata: map[string]any{"sourceIP": "192.168.1.137"}},
		{ID: "mac", Chains: []string{"old-node", group}},
		{ID: "", Chains: []string{"old-node", group}},
	}}
	server.fetchConnections = func(context.Context, config.Config) (mihomo.ConnectionsSnapshot, error) { return snapshot, nil }
	var closedIDs []string
	server.closeConnections = func(_ context.Context, _ config.Config, ids []string) (int, error) {
		closedIDs = append([]string(nil), ids...)
		return len(ids), nil
	}
	endpoint := "/api/v1/policies/" + url.PathEscape(group) + "/connections/refresh"
	response := performAuthorized(server, http.MethodPost, endpoint, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !reflect.DeepEqual(closedIDs, []string{"mac", "inherit", "nested-device", "ipv6", "already-new"}) {
		t.Fatalf("closed IDs = %#v", closedIDs)
	}
	var payload ConnectionRefreshResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Scope != connectionRefreshScopePolicy || payload.PolicyGroup != group || payload.DeviceID != "" || payload.MatchedConnections != 5 || payload.ClosedConnections != 5 {
		t.Fatalf("refresh response = %#v", payload)
	}
	// A later click must use a fresh snapshot, including newly established sessions.
	snapshot.Connections = []mihomo.Connection{{ID: "reconnected", Chains: []string{"new-node", group}}}
	response = performAuthorized(server, http.MethodPost, endpoint, nil)
	if response.Code != http.StatusOK || !reflect.DeepEqual(closedIDs, []string{"reconnected"}) {
		t.Fatalf("repeat refresh status=%d ids=%v", response.Code, closedIDs)
	}
	snapshot.Connections = nil
	response = performAuthorized(server, http.MethodPost, endpoint, nil)
	if response.Code != http.StatusOK || len(closedIDs) != 0 || !strings.Contains(response.Body.String(), `"closed_connections":0`) {
		t.Fatalf("empty refresh status=%d body=%s ids=%v", response.Code, response.Body.String(), closedIDs)
	}
}

func TestPolicyConnectionRefreshRejectsUnavailableAndInternalGroups(t *testing.T) {
	server := newTestServer(t)
	server.fetchProxyGroups = func(context.Context, config.Config) ([]mihomo.ProxyGroup, error) {
		return []mihomo.ProxyGroup{{Name: "Main", Type: "Selector", Selected: "Proxy-A"}}, nil
	}
	server.fetchConnections = func(context.Context, config.Config) (mihomo.ConnectionsSnapshot, error) {
		t.Fatal("invalid group must not read or close connections")
		return mihomo.ConnectionsSnapshot{}, nil
	}
	for _, group := range []string{"missing", "Proxy-A", mihomo.LocalRoutingGlobalGroup, " "} {
		response := performAuthorized(server, http.MethodPost, "/api/v1/policies/"+url.PathEscape(group)+"/connections/refresh", nil)
		if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid_policy_group") {
			t.Fatalf("group=%q status=%d body=%s", group, response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:61767/api/v1/policies/Main/connections/refresh", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPolicyConnectionRefreshReportsFailuresWithoutWideningScope(t *testing.T) {
	for _, failure := range []string{"policies", "snapshot", "partial-close"} {
		t.Run(failure, func(t *testing.T) {
			server := newTestServer(t)
			server.fetchProxyGroups = func(context.Context, config.Config) ([]mihomo.ProxyGroup, error) {
				if failure == "policies" {
					return nil, errors.New("core unavailable")
				}
				return []mihomo.ProxyGroup{{Name: "Main", Type: "Selector"}}, nil
			}
			server.fetchConnections = func(context.Context, config.Config) (mihomo.ConnectionsSnapshot, error) {
				if failure == "snapshot" {
					return mihomo.ConnectionsSnapshot{}, errors.New("snapshot unavailable")
				}
				return mihomo.ConnectionsSnapshot{Connections: []mihomo.Connection{{ID: "one", Chains: []string{"Main"}}, {ID: "two", Chains: []string{"Main"}}, {ID: "other", Chains: []string{"Other"}}}}, nil
			}
			server.closeConnections = func(_ context.Context, _ config.Config, ids []string) (int, error) {
				if failure != "partial-close" || !reflect.DeepEqual(ids, []string{"one", "two"}) {
					t.Fatalf("unexpected close: %v", ids)
				}
				return 1, errors.New("core disconnected")
			}
			response := performAuthorized(server, http.MethodPost, "/api/v1/policies/Main/connections/refresh", nil)
			if response.Code != http.StatusBadGateway {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if failure == "partial-close" && !strings.Contains(response.Body.String(), "closed 1 of 2") {
				t.Fatalf("partial close lost: %s", response.Body.String())
			}
		})
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
