package dhcp

import (
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
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderConfigWithDNSUpstream(t *testing.T) {
	cfg := config.Default()
	cfg.DNS.Upstream = "127.0.0.1#1053"
	paths := runtime.NewPaths(cfg)
	rendered, err := RenderConfig(cfg, paths)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	for _, want := range []string{
		"no-resolv",
		"server=127.0.0.1#1053",
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
