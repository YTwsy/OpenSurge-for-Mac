package mihomo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
		"geox-url:",
		"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.metadb",
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
  listen: 127.0.0.1:5335
  ipv6: true
  enhanced-mode: redir-host
  fake-ip-range: 198.19.0.1/16
  default-nameserver:
    - system
  nameserver:
    - https://dns.example/dns-query
  nameserver-policy:
    "+.nodes.example":
      - https://nodes-dns.example/dns-query
  fake-ip-filter:
    - "*.lan"
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
		"listen: 0.0.0.0:1053",
		"ipv6: false",
		"enhanced-mode: fake-ip",
		"fake-ip-range: 198.18.0.1/16",
		"default-nameserver:",
		"- system",
		"- https://dns.example/dns-query",
		"nameserver-policy:",
		`"+.nodes.example":`,
		"- https://nodes-dns.example/dns-query",
		"fake-ip-filter:",
		`- "*.lan"`,
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
		"listen: 127.0.0.1:5335",
		"ipv6: true",
		"enhanced-mode: redir-host",
		"fake-ip-range: 198.19.0.1/16",
		"open-surge-egress",
	} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("rendered config kept unwanted profile/default value %q:\n%s", notWant, rendered)
		}
	}
}

func TestRenderConfigRejectsMalformedImportedDNS(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	body := `dns:
  - https://dns.example/dns-query
rules:
  - MATCH,DIRECT
`
	if err := os.WriteFile(profilePath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = profilePath
	_, err := RenderConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "dns must be a mapping") {
		t.Fatalf("RenderConfig() error = %v", err)
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

func TestRenderConfigWithDevicePolicyOverlayPreservesImportedRuleOrder(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	policyPath := filepath.Join(dir, "devices.json")
	profile := `proxies: []
proxy-groups:
  - name: Global
    type: select
    proxies:
      - DIRECT
rules:
  - DOMAIN-SUFFIX,global.example,Global
  - MATCH,DIRECT
`
	policy := `{
  "rule_sets": [{"id":"streaming","behavior":"domain","payload":["netflix.com"]}],
  "profiles": [{
    "id":"home",
    "default_policies":["DIRECT","Global"],
    "rules":[
      {"id":"block-video","match":{"domains":["youtube.com"],"protocols":["tcp"]},"action":"REJECT"},
      {"id":"streaming","match":{"rule_sets":["streaming"]},"policies":["Global","DIRECT"]}
    ]
  }],
  "devices": [{"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"home"}]
}`
	if err := os.WriteFile(profilePath, []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = profilePath
	cfg.DevicePolicy.File = policyPath
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	for _, want := range []string{
		"  - name: device/phone/default",
		"  - name: device/phone/streaming",
		"  open-surge-ruleset-streaming:",
		"    type: inline",
		"    behavior: domain",
		"      - \"netflix.com\"",
		"AND,((SRC-IP-CIDR,192.168.50.101/32),(DOMAIN-SUFFIX,youtube.com),(NETWORK,tcp)),REJECT",
		"AND,((SRC-IP-CIDR,192.168.50.101/32),(RULE-SET,open-surge-ruleset-streaming)),device/phone/streaming",
		"SRC-IP-CIDR,192.168.50.101/32,device/phone/default",
		"MATCH,DIRECT",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
	assertOrdered(t, rendered,
		"AND,((SRC-IP-CIDR,192.168.50.101/32),(DOMAIN-SUFFIX,youtube.com),(NETWORK,tcp)),REJECT",
		"DOMAIN-SUFFIX,global.example,Global",
		"SRC-IP-CIDR,192.168.50.101/32,device/phone/default",
		"MATCH,DIRECT",
	)
}

func TestRenderConfigWithDevicePolicyOverlayMatchesImportedSectionIndentation(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	policyPath := filepath.Join(dir, "devices.json")
	profile := `proxies: []
proxy-groups:
    - name: Global
      type: select
      proxies:
        - DIRECT
rule-providers:
    imported:
      type: inline
      behavior: domain
      payload:
        - imported.example
rules:
    - 'RULE-SET,imported,Global'
    - 'MATCH,DIRECT'
`
	policy := `{
  "rule_sets": [{"id":"streaming","behavior":"domain","payload":["netflix.com"]}],
  "profiles": [{
    "id":"home",
    "default_policies":["DIRECT","Global"],
    "rules":[{"id":"streaming","match":{"rule_sets":["streaming"]},"policies":["Global","DIRECT"]}]
  }],
  "devices": [{"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"home"}]
}`
	if err := os.WriteFile(profilePath, []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = profilePath
	cfg.DevicePolicy.File = policyPath
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if err := yaml.Unmarshal([]byte(rendered), &map[string]any{}); err != nil {
		t.Fatalf("rendered config is invalid YAML: %v\n%s", err, rendered)
	}
	for _, want := range []string{
		"    - name: device/phone/default",
		"    - name: device/phone/streaming",
		"    open-surge-ruleset-streaming:",
		"    - SRC-IP-CIDR,192.168.50.101/32,device/phone/default",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing imported indentation %q:\n%s", want, rendered)
		}
	}
	assertOrdered(t, rendered,
		"AND,((SRC-IP-CIDR,192.168.50.101/32),(RULE-SET,open-surge-ruleset-streaming)),device/phone/streaming",
		"RULE-SET,imported,Global",
		"SRC-IP-CIDR,192.168.50.101/32,device/phone/default",
		"'MATCH,DIRECT'",
	)
}

func TestRenderConfigRejectsImportedRuleAfterTerminalMatchWhenDevicePolicyEnabled(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	policyPath := filepath.Join(dir, "devices.json")
	if err := os.WriteFile(profilePath, []byte("rules:\n  - MATCH,DIRECT\n  - DOMAIN,example.com,DIRECT\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, []byte(`{
  "profiles":[{"id":"default","default_policies":["DIRECT"]}],
  "devices":[{"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"default"}]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = profilePath
	cfg.DevicePolicy.File = policyPath
	_, err := RenderConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "MATCH rule must be terminal") {
		t.Fatalf("RenderConfig() error = %v", err)
	}
}

func TestRenderConfigRejectsImportedPolicyNamespaceCollisionsAndUnknownTargets(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		policy  string
		want    string
	}{
		{
			name: "generated group collision",
			profile: `proxy-groups:
  - name: device/phone/default
    type: select
    proxies: [DIRECT]
rules:
  - MATCH,DIRECT
`,
			policy: `{"profiles":[{"id":"home","default_policies":["DIRECT"]}],"devices":[{"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"home"}]}`,
			want:   "occupies reserved device/ namespace",
		},
		{
			name: "unknown policy target",
			profile: `proxies: []
rules:
  - MATCH,DIRECT
`,
			policy: `{"profiles":[{"id":"home","default_policies":["Missing"]}],"devices":[{"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"home"}]}`,
			want:   "unknown imported proxy or group \"Missing\"",
		},
		{
			name: "generated provider collision",
			profile: `rule-providers:
  open-surge-ruleset-media:
    type: inline
    behavior: domain
    payload: [example.com]
rules:
  - MATCH,DIRECT
`,
			policy: `{"rule_sets":[{"id":"media","behavior":"domain","payload":["example.com"]}],"profiles":[{"id":"home","default_policies":["DIRECT"],"rules":[{"id":"media","match":{"rule_sets":["media"]},"action":"DIRECT"}]}],"devices":[{"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"home"}]}`,
			want:   "occupies reserved open-surge-ruleset- namespace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			profilePath := filepath.Join(dir, "profile.yaml")
			policyPath := filepath.Join(dir, "policy.json")
			if err := os.WriteFile(profilePath, []byte(tt.profile), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(policyPath, []byte(tt.policy), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg := config.Default()
			cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
			cfg.Mihomo.Profile = profilePath
			cfg.DevicePolicy.File = policyPath
			if _, err := RenderConfig(cfg); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RenderConfig() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func assertOrdered(t *testing.T, value string, ordered ...string) {
	t.Helper()
	position := -1
	for _, part := range ordered {
		next := strings.Index(value, part)
		if next < 0 || next <= position {
			t.Fatalf("expected ordered %q after offset %d:\n%s", part, position, value)
		}
		position = next
	}
}
