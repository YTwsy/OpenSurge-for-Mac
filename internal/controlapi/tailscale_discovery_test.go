package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/macosnetwork"
	"open-mihomo-gateway/internal/mihomo"
)

func TestParseLocalTailscaleStatusBuildsSafeSuggestions(t *testing.T) {
	result, err := parseLocalTailscaleStatus([]byte(`{
  "BackendState": "Running",
  "MagicDNSSuffix": "example.ts.net",
  "CurrentTailnet": {"Name": "Example", "MagicDNSEnabled": true, "MagicDNSSuffix": "example.ts.net"},
  "Self": {"ID": "self", "HostName": "gateway-mac", "DNSName": "gateway-mac.example.ts.net.", "TailscaleIPs": ["100.64.0.1"]},
  "Peer": {
    "nodekey:offline": {
      "ID": "offline", "HostName": "Offline Phone", "DNSName": "offline-phone.example.ts.net.",
      "TailscaleIPs": ["100.82.10.7", "fd7a:115c:a1e0::7"],
      "AllowedIPs": ["100.82.10.7/32", "fd7a:115c:a1e0::7/128"],
      "Online": false
    },
    "nodekey:router": {
      "ID": "router", "HostName": "Home Router", "DNSName": "home-router.example.ts.net.",
      "TailscaleIPs": ["100.90.3.4"],
      "AllowedIPs": ["100.90.3.4/32", "10.20.0.0/16", "192.168.80.0/24", "0.0.0.0/0", "203.0.113.0/24"],
      "Online": true, "ExitNodeOption": true
    }
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Available || result.BackendState != "Running" || !result.MagicDNS || result.MagicDNSSuffix != "example.ts.net" {
		t.Fatalf("discovery = %#v", result)
	}
	if result.Self == nil || result.Self.DNSName != "gateway-mac.example.ts.net" {
		t.Fatalf("self = %#v", result.Self)
	}
	if len(result.Peers) != 2 || result.Peers[0].ID != "router" || !result.Peers[0].ExitNodeOption {
		t.Fatalf("peers = %#v", result.Peers)
	}
	if got := result.Peers[0].SubnetRoutes; len(got) != 2 || got[0] != "10.20.0.0/16" || got[1] != "192.168.80.0/24" {
		t.Fatalf("subnet routes = %#v", got)
	}
	if got := result.Peers[1].TailscaleIPs; len(got) != 2 || got[0] != "100.82.10.7" || got[1] != "fd7a:115c:a1e0::7" {
		t.Fatalf("peer IPs = %#v", got)
	}
}

func TestTailscaleDiscoveryEndpointCachesSuggestionsForDisconnectedApp(t *testing.T) {
	server := newTestServer(t)
	server.discoverTailscale = func(context.Context) (TailscaleDiscoveryResponse, error) {
		return TailscaleDiscoveryResponse{Available: true, BackendState: "Running", MagicDNS: true, MagicDNSSuffix: "example.ts.net", Peers: []TailscaleDiscoveredNode{{ID: "phone", Name: "Phone", TailscaleIPs: []string{"100.82.10.7"}}}}, nil
	}
	response := performAuthorized(server, http.MethodGet, "/api/v1/tailscale/discovery", nil)
	if response.Code != http.StatusOK || response.Body.String() == "" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); !containsAll(body, `"available":true`, `"magic_dns_suffix":"example.ts.net"`, `"name":"Phone"`) {
		t.Fatalf("unexpected discovery response: %s", body)
	}

	server.discoverTailscale = func(context.Context) (TailscaleDiscoveryResponse, error) {
		return TailscaleDiscoveryResponse{}, errors.New("daemon unavailable\nretry later")
	}
	response = performAuthorized(server, http.MethodGet, "/api/v1/tailscale/discovery", nil)
	if response.Code != http.StatusOK || !containsAll(response.Body.String(), `"available":true`, `"cached":true`, `"backend_state":"Unavailable"`, `"magic_dns_suffix":"example.ts.net"`, `"name":"Phone"`, `daemon unavailable retry later`) {
		t.Fatalf("cached failure status=%d body=%s", response.Code, response.Body.String())
	}
	var cached TailscaleDiscoveryResponse
	if err := json.NewDecoder(response.Body).Decode(&cached); err != nil {
		t.Fatal(err)
	}
	if cached.CachedAt == nil || cached.CachedAt.IsZero() {
		t.Fatalf("cached_at = %#v", cached.CachedAt)
	}
	info, err := os.Stat(filepath.Join(server.store.Dir(), "tailscale-discovery.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode = %v", info.Mode().Perm())
	}
}

func TestTailscaleDiscoveryEndpointKeepsGracefulFailureWithoutCache(t *testing.T) {
	server := newTestServer(t)
	server.discoverTailscale = func(context.Context) (TailscaleDiscoveryResponse, error) {
		return TailscaleDiscoveryResponse{}, errors.New("daemon unavailable\nretry later")
	}
	response := performAuthorized(server, http.MethodGet, "/api/v1/tailscale/discovery", nil)
	if response.Code != http.StatusOK || !containsAll(response.Body.String(), `"available":false`, `"peers":[]`, `daemon unavailable retry later`) {
		t.Fatalf("graceful failure status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestTailscaleSubnetRouteConflictsRequireExactForeignTUNRoute(t *testing.T) {
	server := newTestServer(t)
	server.fetchTUNRuntime = func(context.Context, config.Config) (mihomo.TUNRuntimeState, error) {
		return mihomo.TUNRuntimeState{Enabled: true, Device: "utun123"}, nil
	}
	server.lookupRoute = func(_ context.Context, destination string) (macosnetwork.RouteSelection, error) {
		switch destination {
		case "10.20.0.0":
			return macosnetwork.RouteSelection{Interface: "utun5", Prefix: "10.20.0.0/16"}, nil
		case "10.30.0.0":
			return macosnetwork.RouteSelection{Interface: "utun123", Prefix: "10.30.0.0/16"}, nil
		case "10.40.0.0":
			return macosnetwork.RouteSelection{Interface: "en0", Prefix: "10.40.0.0/16"}, nil
		case "10.50.0.0":
			return macosnetwork.RouteSelection{Interface: "utun5", Prefix: "0.0.0.0/0"}, nil
		default:
			return macosnetwork.RouteSelection{}, errors.New("not found")
		}
	}
	conflicts := server.tailscaleSubnetRouteConflicts(t.Context(), []tailscaleSubnetRouteCandidate{
		{Route: "10.20.0.0/16", PeerID: "router", PeerName: "Home Router"},
		{Route: "10.30.0.0/16"},
		{Route: "10.40.0.0/16"},
		{Route: "10.50.0.0/16"},
	})
	if len(conflicts) != 1 || conflicts[0].Route != "10.20.0.0/16" || conflicts[0].Interface != "utun5" || conflicts[0].PeerName != "Home Router" {
		t.Fatalf("route conflicts = %#v", conflicts)
	}
}

func TestTailscaleDiscoveryEndpointReportsNativeRouteConflict(t *testing.T) {
	server := newTestServer(t)
	server.discoverTailscale = func(context.Context) (TailscaleDiscoveryResponse, error) {
		return TailscaleDiscoveryResponse{Available: true, Peers: []TailscaleDiscoveredNode{{ID: "router", Name: "Home Router", SubnetRoutes: []string{"192.168.64.0/24"}}}}, nil
	}
	server.fetchTUNRuntime = func(context.Context, config.Config) (mihomo.TUNRuntimeState, error) {
		return mihomo.TUNRuntimeState{Enabled: true, Device: "utun123"}, nil
	}
	server.lookupRoute = func(context.Context, string) (macosnetwork.RouteSelection, error) {
		return macosnetwork.RouteSelection{Interface: "utun5", Prefix: "192.168.64.0/24"}, nil
	}
	response := performAuthorized(server, http.MethodGet, "/api/v1/tailscale/discovery", nil)
	if response.Code != http.StatusOK || !containsAll(response.Body.String(), `"subnet_route_conflicts":[`, `"route":"192.168.64.0/24"`, `"interface":"utun5"`, `"peer_name":"Home Router"`) {
		t.Fatalf("conflict discovery status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLocalTailscaleStatusCommandForcesCLIMode(t *testing.T) {
	t.Setenv("TAILSCALE_BE_CLI", "0")
	command := localTailscaleStatusCommand(t.Context(), "/Applications/Tailscale.app/Contents/MacOS/Tailscale")
	if len(command.Args) != 3 || command.Args[1] != "status" || command.Args[2] != "--json" {
		t.Fatalf("args = %#v", command.Args)
	}
	for index := len(command.Env) - 1; index >= 0; index-- {
		if value, found := strings.CutPrefix(command.Env[index], "TAILSCALE_BE_CLI="); found {
			if value != "1" {
				t.Fatalf("effective TAILSCALE_BE_CLI = %q", value)
			}
			return
		}
	}
	t.Fatal("TAILSCALE_BE_CLI is missing")
}

func containsAll(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if !strings.Contains(value, candidate) {
			return false
		}
	}
	return true
}
