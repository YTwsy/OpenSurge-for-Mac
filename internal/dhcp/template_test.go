package dhcp

import (
	"os"
	"os/exec"
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

func TestRenderConfigWithDownstreamIPv6RAAndRDNSS(t *testing.T) {
	cfg := config.Default()
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Always
	rendered, err := RenderConfig(cfg, runtime.NewPaths(cfg))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"enable-ra",
		"dhcp-range=fdfe:dcba:9878::,ra-stateless,64,12h",
		"dhcp-option=option6:dns-server,[fe80::]",
		"ra-param=en0,high,20,60",
		"listen-address=fdfe:dcba:9878::1",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered IPv6 dnsmasq config missing %q:\n%s", want, rendered)
		}
	}
	// dnsmasq replaces [fe80::] with the interface's link-local address for
	// DHCPv6 and RDNSS. An explicit value is required because its automatic
	// DHCPv6 option otherwise prefers the downstream ULA when one is present.
	if strings.Contains(rendered, "option6:dns-server,[fdfe:") {
		t.Fatalf("rendered IPv6 dnsmasq config advertises a ULA DNS endpoint:\n%s", rendered)
	}
}

func TestDNSMasqAcceptsDownstreamIPv6RAConfig(t *testing.T) {
	binary := os.Getenv("OPENSURGE_TEST_DNSMASQ")
	if binary == "" {
		t.Skip("OPENSURGE_TEST_DNSMASQ is not set")
	}
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Always
	paths := runtime.NewPaths(cfg)
	rendered, err := RenderConfig(cfg, paths)
	if err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(cfg.Runtime.Dir, "dnsmasq.conf")
	if err := os.WriteFile(conf, []byte(rendered), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(binary, "--test", "--conf-file="+conf).CombinedOutput(); err != nil {
		t.Fatalf("dnsmasq rejected IPv6 RA config: %v: %s", err, strings.TrimSpace(string(output)))
	}
}
