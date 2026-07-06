package mihomo

import (
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestRenderConfig(t *testing.T) {
	cfg := config.Default()
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	for _, want := range []string{
		"mixed-port: 7890",
		"external-controller: 127.0.0.1:9090",
		"enhanced-mode: fake-ip",
		"- MATCH,DIRECT",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "proxy-groups:") {
		t.Fatalf("rendered config contains an unnecessary DIRECT proxy group:\n%s", rendered)
	}
	if strings.Contains(rendered, "open-surge-egress") {
		t.Fatalf("rendered config enables upstream proxy by default:\n%s", rendered)
	}
	if strings.Contains(rendered, "redir-port:") {
		t.Fatalf("rendered config enables redir-port by default:\n%s", rendered)
	}
	if strings.Contains(rendered, "tun:") {
		t.Fatalf("rendered config enables tun by default:\n%s", rendered)
	}
}

func TestRenderConfigWithUpstreamProxy(t *testing.T) {
	cfg := config.Default()
	cfg.UpstreamProxy.Enabled = true
	cfg.UpstreamProxy.Name = "real-device-egress"
	cfg.UpstreamProxy.Type = "http"
	cfg.UpstreamProxy.Server = "127.0.0.1"
	cfg.UpstreamProxy.Port = 18080
	cfg.UpstreamProxy.MatchDomain = "example.com"
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	for _, want := range []string{
		"proxies:",
		`  - name: "real-device-egress"`,
		"    type: http",
		`    server: "127.0.0.1"`,
		"    port: 18080",
		"proxy-groups:",
		"  - name: open-surge-egress",
		`      - "real-device-egress"`,
		"- DOMAIN,example.com,open-surge-egress",
		"- MATCH,DIRECT",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "proxies: []") {
		t.Fatalf("rendered config still emits empty proxies list:\n%s", rendered)
	}
	if strings.Contains(rendered, "18080proxy-groups") {
		t.Fatalf("rendered config glues port and proxy group:\n%s", rendered)
	}
}

func TestRenderConfigNeverEmitsRedirPort(t *testing.T) {
	cfg := config.Default()
	cfg.Mihomo.RedirPort = 7892
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if strings.Contains(rendered, "redir-port:") {
		t.Fatalf("rendered config emits unsupported redir-port:\n%s", rendered)
	}
}

func TestRenderConfigWithTUN(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "bridge100"
	cfg.Gateway.UpstreamInterface = "en0"
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Transparent.TUNDevice = "utun123"
	cfg.Transparent.TUNStack = "mixed"
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	for _, want := range []string{
		"interface-name: en0",
		"tun:",
		"  enable: true",
		"  stack: mixed",
		"  device: utun123",
		"  auto-route: true",
		"  dns-hijack:",
		"    - any:53",
		"  route-exclude-address:",
		"    - 192.168.50.0/24",
		"    - 192.168.0.0/16",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
}
