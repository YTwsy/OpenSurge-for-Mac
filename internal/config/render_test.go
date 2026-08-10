package config

import (
	"os"
	"path/filepath"
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
	if loaded.Gateway.Mode != cfg.Gateway.Mode || loaded.DHCP.RangeStart != cfg.DHCP.RangeStart || loaded.DHCP.BypassGateway != cfg.DHCP.BypassGateway || len(loaded.DHCP.BypassDNS) != 2 || !loaded.LocalSystemProxy.Enabled {
		t.Fatalf("round trip mismatch: %#v", loaded)
	}
}
