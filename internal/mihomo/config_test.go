package mihomo

import (
	"os"
	"path/filepath"
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
		"profile:",
		"  store-selected: true",
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

func TestRenderConfigWithImportedProfileOverlay(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	body := `allow-lan: false
bind-address: 127.0.0.1
external-controller: 127.0.0.1:9999
dns:
  enable: false
proxies:
  - name: Imported
    type: socks5
    server: 203.0.113.10
    port: 1080
proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - Imported
rules:
  - DOMAIN-SUFFIX,example.com,Proxy
  - MATCH,DIRECT
tun:
  enable: false
`
	if err := os.WriteFile(profilePath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = profilePath
	cfg.Mihomo.MixedPort = 17890
	cfg.Mihomo.APIAddr = "127.0.0.1:19090"
	cfg.Transparent.Mode = config.TransparentModeTUN
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	for _, want := range []string{
		"mixed-port: 17890",
		"allow-lan: true",
		"bind-address: \"*\"",
		"external-controller: 127.0.0.1:19090",
		"enhanced-mode: fake-ip",
		"tun:",
		"  enable: true",
		"proxies:",
		"  - name: Imported",
		"proxy-groups:",
		"rules:",
		"- DOMAIN-SUFFIX,example.com,Proxy",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
	for _, notWant := range []string{
		"allow-lan: false",
		"external-controller: 127.0.0.1:9999",
		"enable: false",
		"open-surge-egress",
	} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("rendered config kept unwanted profile/default value %q:\n%s", notWant, rendered)
		}
	}
}

func TestRenderConfigWithImportedExampleProfile(t *testing.T) {
	cfg := config.Default()
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = filepath.Join("..", "..", "examples", "mihomo-profile.example.yaml")

	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	for _, want := range []string{
		`- name: "demo-proxy"`,
		`- name: "Proxy"`,
		"- DOMAIN,example.com,Proxy",
		"- MATCH,DIRECT",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
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
