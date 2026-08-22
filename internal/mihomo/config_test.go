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
		"  store-fake-ip: true",
		"geox-url:",
		"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.metadb",
		"enhanced-mode: fake-ip",
		`name: "open-surge/mac-mode-tcp"`,
		`name: "open-surge/mac-mode-udp"`,
		`- "PASS"`,
		"hidden: true",
		"- MATCH,DIRECT",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "open-surge-egress") {
		t.Fatalf("rendered config enables upstream proxy by default:\n%s", rendered)
	}
	if strings.Contains(rendered, LocalRoutingGlobalGroup) {
		t.Fatalf("rendered config exposes global mode without a proxy candidate:\n%s", rendered)
	}
	if strings.Contains(rendered, "redir-port:") {
		t.Fatalf("rendered config enables redir-port by default:\n%s", rendered)
	}
	if strings.Contains(rendered, "tun:") {
		t.Fatalf("rendered config enables tun by default:\n%s", rendered)
	}
}

func TestRenderConfigCanDisableFakeIPPersistence(t *testing.T) {
	cfg := config.Default()
	cfg.Mihomo.StoreFakeIP = false
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if !strings.Contains(rendered, "  store-fake-ip: false") {
		t.Fatalf("rendered config did not disable fake-IP persistence:\n%s", rendered)
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

func TestRenderConfigSeparatesInheritedAndDedicatedDeviceEgress(t *testing.T) {
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
  "profiles": [{
    "id":"home",
    "default_policies":["DIRECT","Global"],
    "rules":[{"id":"private","match":{"domains":["device.example"]},"action":"REJECT"}]
  }],
  "devices": [
    {"id":"follower","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"home","egress_mode":"inherit_global"},
    {"id":"dedicated","mac":"aa:bb:cc:dd:ee:02","ipv4":"192.168.50.102","profile":"home","egress_mode":"dedicated"}
  ]
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
		t.Fatal(err)
	}
	if strings.Contains(rendered, "device/follower/default") {
		t.Fatalf("inherit-global device unexpectedly rendered a default selector:\n%s", rendered)
	}
	for _, want := range []string{
		"  - name: device/dedicated/default",
		"AND,((SRC-IP-CIDR,192.168.50.102/32),(IP-CIDR,192.168.0.0/16)),DIRECT",
		"AND,((SRC-IP-CIDR,192.168.50.102/32),(DOMAIN-SUFFIX,device.example)),REJECT",
		"SRC-IP-CIDR,192.168.50.102/32,device/dedicated/default",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
	assertOrdered(t, rendered,
		"AND,((SRC-IP-CIDR,192.168.50.102/32),(IP-CIDR,192.168.0.0/16)),DIRECT",
		"AND,((SRC-IP-CIDR,192.168.50.102/32),(DOMAIN-SUFFIX,device.example)),REJECT",
		"SRC-IP-CIDR,192.168.50.102/32,device/dedicated/default",
		"DOMAIN-SUFFIX,global.example,Global",
		"MATCH,DIRECT",
	)
}

func TestRenderManagedConfigPlacesDedicatedEgressBeforeGlobalMatch(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "devices.json")
	policy := `{
  "profiles":[{"id":"home","default_policies":["DIRECT"]}],
  "devices":[{"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"home","egress_mode":"dedicated"}]
}`
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.DevicePolicy.File = policyPath
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	assertOrdered(t, rendered,
		"AND,((SRC-IP-CIDR,192.168.50.101/32),(IP-CIDR,192.168.0.0/16)),DIRECT",
		"SRC-IP-CIDR,192.168.50.101/32,device/phone/default",
		"MATCH,DIRECT",
	)
}

func TestRenderConfigWithDevicePolicyOverlayNormalizesImportedSectionIndentation(t *testing.T) {
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
		"  - name: device/phone/default",
		"  - name: device/phone/streaming",
		"  open-surge-ruleset-streaming:",
		"  - SRC-IP-CIDR,192.168.50.101/32,device/phone/default",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing normalized indentation %q:\n%s", want, rendered)
		}
	}
	assertOrdered(t, rendered,
		"AND,((SRC-IP-CIDR,192.168.50.101/32),(RULE-SET,open-surge-ruleset-streaming)),device/phone/streaming",
		"RULE-SET,imported,Global",
		"SRC-IP-CIDR,192.168.50.101/32,device/phone/default",
		"'MATCH,DIRECT'",
	)
}

func TestRenderConfigSupportsIssue7FlowStyleRules(t *testing.T) {
	dir := t.TempDir()
	want := []string{
		"DOMAIN-SUFFIX,deeplx.org,DIRECT",
		"DOMAIN-SUFFIX,derp.tailscale.com,DIRECT",
		"DOMAIN,example.com,Proxy",
		"GEOIP,CN,DIRECT",
		"MATCH,Proxy",
	}
	profiles := map[string]string{
		"flow":  `rules: ['DOMAIN-SUFFIX,deeplx.org,DIRECT', 'DOMAIN-SUFFIX,derp.tailscale.com,DIRECT', 'DOMAIN,example.com,Proxy', 'GEOIP,CN,DIRECT', 'MATCH,Proxy']`,
		"block": "rules:\n  - DOMAIN-SUFFIX,deeplx.org,DIRECT\n  - DOMAIN-SUFFIX,derp.tailscale.com,DIRECT\n  - DOMAIN,example.com,Proxy\n  - GEOIP,CN,DIRECT\n  - MATCH,Proxy",
	}
	decodedRules := map[string][]string{}
	for style, profile := range profiles {
		profilePath := filepath.Join(dir, style+".yaml")
		if err := os.WriteFile(profilePath, []byte(profile+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg := config.Default()
		cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
		cfg.Mihomo.Profile = profilePath
		rendered, err := RenderConfig(cfg)
		if err != nil {
			t.Fatalf("RenderConfig(%s) error = %v", style, err)
		}
		var decoded struct {
			Rules []string `yaml:"rules"`
		}
		if err := yaml.Unmarshal([]byte(rendered), &decoded); err != nil {
			t.Fatalf("rendered %s config is invalid YAML: %v\n%s", style, err, rendered)
		}
		decodedRules[style] = decoded.Rules
		if len(decoded.Rules) < len(want) || strings.Join(decoded.Rules[len(decoded.Rules)-len(want):], "\n") != strings.Join(want, "\n") {
			t.Fatalf("%s rules do not preserve imported suffix = %#v, want suffix %#v", style, decoded.Rules, want)
		}
	}
	if strings.Join(decodedRules["flow"], "\n") != strings.Join(decodedRules["block"], "\n") {
		t.Fatalf("flow rules = %#v, block rules = %#v", decodedRules["flow"], decodedRules["block"])
	}
}

func TestRenderConfigComposesDevicePolicyIntoFlowStyleSections(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	policyPath := filepath.Join(dir, "devices.json")
	profile := `proxies: []
proxy-groups: [{name: Global, type: select, proxies: [DIRECT]}]
rule-providers: {imported: {type: inline, behavior: domain, payload: [imported.example]}}
rules: ['RULE-SET,imported,Global', 'MATCH,DIRECT']
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
	var decoded struct {
		ProxyGroups []struct {
			Name    string   `yaml:"name"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
		RuleProviders map[string]any `yaml:"rule-providers"`
		Rules         []string       `yaml:"rules"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &decoded); err != nil {
		t.Fatalf("rendered config is invalid YAML: %v\n%s", err, rendered)
	}
	if len(decoded.ProxyGroups) != 6 {
		t.Fatalf("proxy groups = %#v", decoded.ProxyGroups)
	}
	if decoded.ProxyGroups[0].Name != "Global" ||
		len(decoded.ProxyGroups[0].Proxies) != 1 ||
		decoded.ProxyGroups[0].Proxies[0] != "DIRECT" {
		t.Fatalf("imported flow-style proxy group changed: %#v", decoded.ProxyGroups[0])
	}
	if decoded.RuleProviders["imported"] == nil || decoded.RuleProviders["open-surge-ruleset-streaming"] == nil {
		t.Fatalf("rule providers = %#v", decoded.RuleProviders)
	}
	for _, name := range []string{LocalRoutingGlobalGroup, LocalRoutingTCPGroup, LocalRoutingUDPGroup, "device/phone/default", "device/phone/streaming"} {
		found := false
		for _, group := range decoded.ProxyGroups {
			if group.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("proxy groups missing %q: %#v", name, decoded.ProxyGroups)
		}
	}
	assertOrdered(t, strings.Join(decoded.Rules, "\n"),
		"RULE-SET,open-surge-ruleset-streaming",
		"RULE-SET,imported,Global",
		"SRC-IP-CIDR,192.168.50.101/32,device/phone/default",
		"MATCH,DIRECT",
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

func TestRenderConfigAddsDownstreamIPv6PacketListenerAndDeviceIdentity(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "devices.json")
	policy := `{"profiles":[{"id":"home","default_policies":["DIRECT"]}],"devices":[{"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"home","egress_mode":"dedicated"},{"id":"dormant","mac":"aa:bb:cc:dd:ee:02","ipv4":"192.168.60.101","profile":"home","egress_mode":"dedicated"}]}`
	if err := os.WriteFile(policyPath, []byte(policy), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Runtime.Dir = dir
	cfg.Mihomo.Config = filepath.Join(dir, "mihomo.yaml")
	cfg.DevicePolicy.File = policyPath
	cfg.DNS.IPv6 = true
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Always
	cfg.UpstreamProxy.Enabled = true
	cfg.UpstreamProxy.Name = "lab-socks"
	cfg.UpstreamProxy.Type = "socks5"
	cfg.UpstreamProxy.Server = "127.0.0.1"
	cfg.UpstreamProxy.Port = 18080
	cfg.UpstreamProxy.MatchDomain = "example.com"
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ipv6: true",
		"fake-ip-range6: " + config.MihomoFakeIPv6Range,
		"- " + config.MihomoTUNIPv6,
		"type: opensurge-packet",
		"name: " + config.IPv6PacketListenerName,
		`"aa:bb:cc:dd:ee:01": "device:phone"`,
		"IN-USER,device:phone",
		"IP-CIDR6," + config.DownstreamIPv6Prefix,
		"    udp: true",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered IPv6 mihomo config missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "IP-CIDR6,fc00::/7") {
		t.Fatalf("fake IPv6 ULA would be forced DIRECT:\n%s", rendered)
	}
	for _, dormant := range []string{"aa:bb:cc:dd:ee:02", "device:dormant", "192.168.60.101"} {
		if strings.Contains(rendered, dormant) {
			t.Fatalf("dormant device leaked into IPv4 or IPv6 mihomo policy as %q:\n%s", dormant, rendered)
		}
	}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(rendered), &document); err != nil {
		t.Fatalf("rendered IPv6 config is invalid YAML: %v\n%s", err, rendered)
	}
}

func TestRenderConfigRejectsIPv6BeforeRulesForRouterBypassDevice(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "devices.json")
	policy := `{
  "profiles": [{"id":"home","default_policies":["DIRECT"],"rules":[{"id":"phone-rule","match":{"domains":["phone.example"]},"action":"DIRECT"}]}],
  "devices": [
    {"id":"console","mac":"aa:bb:cc:dd:ee:05","ipv4":"192.168.50.105","profile":"home","gateway_target":"upstream_router","egress_mode":"dedicated"},
    {"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"home","egress_mode":"inherit_global"}
  ]
}`
	if err := os.WriteFile(policyPath, []byte(policy), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Runtime.Dir = dir
	cfg.Mihomo.Config = filepath.Join(dir, "mihomo.yaml")
	cfg.DevicePolicy.File = policyPath
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Always

	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	bypassReject := "AND,((IN-TYPE,TUN),(IN-USER,device:console)),REJECT"
	phoneRule := "AND,((IN-USER,device:phone),(DOMAIN-SUFFIX,phone.example)),DIRECT"
	for _, want := range []string{
		`"aa:bb:cc:dd:ee:05": "device:console"`,
		`"aa:bb:cc:dd:ee:01": "device:phone"`,
		bypassReject,
		phoneRule,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered router-bypass IPv6 config missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "AND,((IN-TYPE,TUN),(IN-USER,device:phone)),REJECT") {
		t.Fatalf("non-bypass device IPv6 was rejected:\n%s", rendered)
	}
	assertOrdered(t, rendered, bypassReject, "IN-TYPE,TUN", phoneRule, "MATCH,DIRECT")

	cfg.Transparent.TUNIPv6 = config.TUNIPv6Off
	rendered, err = RenderConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, bypassReject) || strings.Contains(rendered, "type: opensurge-packet") {
		t.Fatalf("disabled downstream IPv6 emitted packet listener or bypass reject:\n%s", rendered)
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
