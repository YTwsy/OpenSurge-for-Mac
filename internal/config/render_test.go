package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderRoundTrip(t *testing.T) {
	cfg := Default()
	cfg.Gateway.Mode = GatewayModeSameWiFiDHCP
	cfg.Gateway.Interface = "en0"
	cfg.Gateway.UpstreamInterface = "en0"
	cfg.Gateway.LANIP = "192.168.1.20"
	cfg.DHCP.RangeStart = "192.168.1.120"
	cfg.DHCP.RangeEnd = "192.168.1.199"
	cfg.DHCP.BypassGateway = "192.168.1.1"
	cfg.DHCP.BypassDNS = []string{"192.168.1.1", "1.1.1.1"}
	cfg.Transparent.Mode = TransparentModeTUN
	cfg.LocalSystemProxy.Enabled = true
	cfg.Mihomo.StoreFakeIP = false
	cfg.DevicePolicy.ProtectedIPv4 = []string{"192.168.1.1", "192.168.1.21"}
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "devices.json")
	if err := os.WriteFile(policyPath, []byte(`{"devices":[],"profiles":[],"templates":[],"rule_sets":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.DevicePolicy.File = policyPath
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load(Render()) error = %v", err)
	}
	if loaded.Gateway.Mode != cfg.Gateway.Mode || loaded.DHCP.RangeStart != cfg.DHCP.RangeStart || loaded.DHCP.BypassGateway != cfg.DHCP.BypassGateway || len(loaded.DHCP.BypassDNS) != 2 || !loaded.LocalSystemProxy.Enabled || loaded.Mihomo.StoreFakeIP {
		t.Fatalf("round trip mismatch: %#v", loaded)
	}
}

func TestRenderRoundTripPreservesIPv6Controls(t *testing.T) {
	cfg := Default()
	cfg.Transparent.Mode = TransparentModeTUN
	cfg.Transparent.TUNIPv6 = TUNIPv6Always
	cfg.Transparent.IPv6SharedL2Ready = true
	cfg.Transparent.IPv6PacketBrokerBinary = "/tmp/opensurge-network"
	cfg.Transparent.IPv6PacketMTU = 1420
	cfg.DNS.IPv6 = true
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load(Render()) error = %v", err)
	}
	if !loaded.DNS.IPv6 || loaded.Transparent.TUNIPv6 != TUNIPv6Always || !loaded.Transparent.IPv6SharedL2Ready || loaded.Transparent.IPv6PacketBrokerBinary != "/tmp/opensurge-network" || loaded.Transparent.IPv6PacketMTU != 1420 {
		t.Fatalf("IPv6 round trip mismatch: %#v", loaded.Transparent)
	}
}

func TestRenderRoundTripPreservesTailscaleWithoutAuthKey(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.Tailscale = TailscaleConfig{
		Enabled:                true,
		DisplayName:            "Home Tailnet",
		Hostname:               "opensurge-home",
		ControlURL:             "https://headscale.example.com",
		AuthKeyFile:            filepath.Join(dir, "tailscale-auth-key"),
		StateDir:               filepath.Join(dir, "tailscale-state"),
		AcceptRoutes:           true,
		MagicDNSSuffixes:       []string{"home.example.ts.net"},
		PeerCIDRs:              []string{"100.82.10.7/32"},
		SubnetRoutes:           []string{"10.20.0.0/16"},
		AllowMac:               true,
		ExitNode:               "100.90.3.4",
		ExitNodeAllowLANAccess: true,
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load(Render()) error = %v", err)
	}
	if loaded.Tailscale.DisplayName != "Home Tailnet" || loaded.Tailscale.AuthKeyFile != cfg.Tailscale.AuthKeyFile || loaded.Tailscale.StateDir != cfg.Tailscale.StateDir || len(loaded.Tailscale.SubnetRoutes) != 1 || loaded.Tailscale.ExitNode != "100.90.3.4" {
		t.Fatalf("Tailscale round trip mismatch: %#v", loaded.Tailscale)
	}
	if strings.Contains(Render(loaded), "tskey-") {
		t.Fatal("rendered gateway config unexpectedly contains an auth key")
	}
}
