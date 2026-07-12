package dhcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

func TestRenderConfig(t *testing.T) {
	cfg := config.Default()
	paths := runtime.NewPaths(cfg)
	rendered, err := RenderConfig(cfg, paths)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	for _, want := range []string{
		"interface=en0",
		"dhcp-range=192.168.50.100,192.168.50.200,12h",
		"dhcp-option=option:router,192.168.50.1",
		"port=53",
		"no-resolv",
		"server=127.0.0.1#1053",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderConfigMigratesEmptyDNSUpstreamToMihomo(t *testing.T) {
	cfg := config.Default()
	cfg.DNS.Upstream = ""
	rendered, err := RenderConfig(cfg, runtime.NewPaths(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "server="+config.MihomoDNSUpstream) {
		t.Fatalf("rendered config does not use mihomo DNS fallback:\n%s", rendered)
	}
}

func TestRenderConfigWithDevicePolicyReservations(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "devices.json")
	policy := `{
  "profiles":[{"id":"default","default_policies":["DIRECT"]}],
  "devices":[
    {"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"default"},
    {"id":"tablet","mac":"aa:bb:cc:dd:ee:02","ipv4":"192.168.50.102","profile":"default"}
  ]
}`
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.DevicePolicy.File = policyPath
	rendered, err := RenderConfig(cfg, runtime.NewPaths(cfg))
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	for _, want := range []string{
		"dhcp-host=aa:bb:cc:dd:ee:01,192.168.50.101",
		"dhcp-host=aa:bb:cc:dd:ee:02,192.168.50.102",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderConfigWithDNSUpstream(t *testing.T) {
	cfg := config.Default()
	cfg.DNS.Upstream = "1.1.1.1"
	paths := runtime.NewPaths(cfg)
	rendered, err := RenderConfig(cfg, paths)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	for _, want := range []string{
		"no-resolv",
		"server=1.1.1.1",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderConfigSameLANDNSOnly(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Mode = config.GatewayModeSameLAN
	cfg.DHCP.Enabled = false
	cfg.DNS.Upstream = "127.0.0.1#1053"
	paths := runtime.NewPaths(cfg)
	rendered, err := RenderConfig(cfg, paths)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	for _, want := range []string{
		"interface=en0",
		"port=53",
		"listen-address=192.168.50.1",
		"server=127.0.0.1#1053",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
	for _, notWant := range []string{
		"dhcp-range=",
		"dhcp-option=option:router",
		"log-dhcp",
		"dhcp-leasefile=",
	} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("rendered DNS-only config contains %q:\n%s", notWant, rendered)
		}
	}
}

func TestRenderConfigSameWiFiDHCP(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Mode = config.GatewayModeSameWiFiDHCP
	cfg.Gateway.Interface = "en0"
	cfg.Gateway.LANIP = "192.168.1.20"
	cfg.DHCP.Enabled = true
	cfg.DHCP.RangeStart = "192.168.1.120"
	cfg.DHCP.RangeEnd = "192.168.1.199"
	cfg.DNS.Listen = "192.168.1.20"
	cfg.DNS.Upstream = "127.0.0.1#1053"
	paths := runtime.NewPaths(cfg)
	rendered, err := RenderConfig(cfg, paths)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	for _, want := range []string{
		"interface=en0",
		"dhcp-range=192.168.1.120,192.168.1.199,12h",
		"dhcp-option=option:router,192.168.1.20",
		"dhcp-option=option:dns-server,192.168.1.20",
		"log-dhcp",
		"server=127.0.0.1#1053",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
}
