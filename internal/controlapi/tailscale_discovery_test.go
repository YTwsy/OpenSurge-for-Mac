package controlapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
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

func TestTailscaleDiscoveryEndpointReturnsSuggestionsAndGracefulFailure(t *testing.T) {
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
	if response.Code != http.StatusOK || !containsAll(response.Body.String(), `"available":false`, `"peers":[]`, `daemon unavailable retry later`) {
		t.Fatalf("graceful failure status=%d body=%s", response.Code, response.Body.String())
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
