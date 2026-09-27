package mihomo

import (
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
)

func TestRenderTailscaleExitCandidatesForUserSelectors(t *testing.T) {
	const source = `proxies:
  - {name: ProxyA, type: socks5, server: 127.0.0.1, port: 18080}
proxy-providers:
  subscription:
    type: file
    path: ./subscription.yaml
proxy-groups:
  - name: Explicit
    type: select
    proxies: [DIRECT, ProxyA]
  - name: ProviderOnly
    type: select
    use: [subscription]
    default-selected: ProxyA
    empty-fallback: REJECT
  - name: IncludeAll
    type: select
    include-all: true
    filter: '^Region'
    exclude-filter: '^open-surge/'
    exclude-type: Selector
  - name: HiddenUser
    type: select
    hidden: true
    proxies: [DIRECT]
  - name: GLOBAL
    type: select
    proxies: [Explicit]
  - name: Automatic
    type: url-test
    proxies: [ProxyA]
  - name: Failover
    type: fallback
    use: [subscription]
  - name: Balanced
    type: load-balance
    proxies: [ProxyA]
rules:
  - DOMAIN-SUFFIX,example.com,Explicit
  - MATCH,DIRECT
`
	for _, tt := range []struct {
		name    string
		enabled bool
		exit    string
		want    bool
	}{
		{name: "enabled Exit", enabled: true, exit: "100.90.3.4", want: true},
		{name: "Tailnet only", enabled: true},
		{name: "disabled with saved Exit", exit: "100.90.3.4"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tailscaleSelectorTestConfig(t, source)
			cfg.Tailscale.Enabled = tt.enabled
			cfg.Tailscale.ExitNode = tt.exit
			cfg.Tailscale.AllowMac = false
			cfg.Tailscale.MagicDNSSuffixes = []string{"home.example.ts.net"}
			cfg.Tailscale.PeerCIDRs = []string{"100.82.10.7/32"}
			rendered, err := RenderConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			groups := renderedTailscaleSelectorGroups(t, rendered)
			for name, original := range map[string][]string{
				"Explicit": {"DIRECT", "ProxyA"}, "ProviderOnly": nil,
				"IncludeAll": nil, "HiddenUser": {"DIRECT"}, "GLOBAL": {"Explicit"},
			} {
				want := append([]string(nil), original...)
				if tt.want {
					want = append(want, config.TailscaleExitGroupName)
				}
				assertTailscaleGroupCandidates(t, groups[name], want)
			}
			assertTailscaleGroupCandidates(t, groups["Automatic"], []string{"ProxyA"})
			assertTailscaleGroupCandidates(t, groups["Failover"], nil)
			assertTailscaleGroupCandidates(t, groups["Balanced"], []string{"ProxyA"})
			provider := groups["ProviderOnly"]
			if provider["default-selected"] != "ProxyA" || provider["empty-fallback"] != "REJECT" || !reflect.DeepEqual(provider["use"], []any{"subscription"}) {
				t.Fatalf("provider selector fields changed: %#v", provider)
			}
			include := groups["IncludeAll"]
			if include["include-all"] != true || include["filter"] != "^Region" || include["exclude-filter"] != "^open-surge/" || include["exclude-type"] != "Selector" {
				t.Fatalf("native include/filter fields changed: %#v", include)
			}
			for name, group := range groups {
				if !IsLocalRoutingGroup(name) {
					continue
				}
				for _, candidate := range tailscaleGroupCandidates(t, group) {
					if candidate == config.TailscaleExitGroupName {
						t.Fatalf("allow_mac=false unexpectedly added direct Exit candidate to %s", name)
					}
				}
			}
			if tt.want {
				assertTailscaleGroupCandidates(t, groups[config.TailscaleExitGroupName], []string{config.TailscaleProxyName})
			}
			if tt.enabled {
				assertOrdered(t, rendered, "DOMAIN-SUFFIX,home.example.ts.net,REJECT", "DOMAIN-SUFFIX,example.com,Explicit")
				if strings.Contains(rendered, ")),"+config.TailscaleProxyName) {
					t.Fatalf("adding public Exit candidates expanded Tailnet source authorization:\n%s", rendered)
				}
			}
			unchanged, err := os.ReadFile(cfg.Mihomo.Profile)
			if err != nil || string(unchanged) != source {
				t.Fatalf("source profile was changed: %v", err)
			}
		})
	}
}

func TestTailscaleExitCandidatesDoNotMutateSharedAliasesOrTemplates(t *testing.T) {
	const source = `proxy-groups:
  - &manual
    name: Manual
    type: select
    proxies: &shared [DIRECT, REJECT]
  - name: SharedAutomatic
    type: url-test
    proxies: *shared
  - <<: *manual
    name: InheritedManual
  - <<: *manual
    name: InheritedAutomatic
    type: fallback
rules: ['MATCH,DIRECT']
`
	cfg := tailscaleSelectorTestConfig(t, source)
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	groups := renderedTailscaleSelectorGroups(t, rendered)
	for _, name := range []string{"Manual", "InheritedManual"} {
		assertTailscaleGroupCandidates(t, groups[name], []string{"DIRECT", "REJECT", config.TailscaleExitGroupName})
	}
	for _, name := range []string{"SharedAutomatic", "InheritedAutomatic"} {
		assertTailscaleGroupCandidates(t, groups[name], []string{"DIRECT", "REJECT"})
	}
}

func TestTailscaleExitCandidatesIncludeComposedOverlaySelectors(t *testing.T) {
	const source = "proxy-groups:\n  - {name: Original, type: select, proxies: [DIRECT]}\nrules: ['MATCH,DIRECT']\n"
	overlay, err := ParseProfileOverlay([]byte(`schema-version: 1
enabled: true
proxy-groups:
  add:
    - {name: Added, type: select, proxies: [Original]}
    - {name: AutoAdded, type: fallback, proxies: [DIRECT]}
  patch:
    - name: Original
      append-proxies: [REJECT]
`))
	if err != nil {
		t.Fatal(err)
	}
	composed, err := ComposeProfileOverlay([]byte(source), overlay)
	if err != nil {
		t.Fatal(err)
	}
	cfg := tailscaleSelectorTestConfig(t, composed.ProfileYAML)
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	groups := renderedTailscaleSelectorGroups(t, rendered)
	assertTailscaleGroupCandidates(t, groups["Original"], []string{"DIRECT", "REJECT", config.TailscaleExitGroupName})
	assertTailscaleGroupCandidates(t, groups["Added"], []string{"Original", config.TailscaleExitGroupName})
	assertTailscaleGroupCandidates(t, groups["AutoAdded"], []string{"DIRECT"})
}

func TestTailscaleExitCandidateInjectionIsIdempotentAndExcludesInternalGroups(t *testing.T) {
	root, err := decodeSingleYAMLMapping([]byte(`proxy-groups:
  - {name: User, type: select, proxies: [DIRECT, open-surge/tailscale-exit]}
  - {name: open-surge/mac-global, type: select, proxies: [DIRECT]}
  - {name: device/phone/default, type: select, proxies: [DIRECT]}
  - {name: open-surge/tailscale-exit, type: select, proxies: [open-surge/tailscale]}
`))
	if err != nil {
		t.Fatal(err)
	}
	profile := importedProfile{sections: map[string]*yaml.Node{"proxy-groups": root.Content[1]}}
	cfg := config.Default()
	cfg.Tailscale.Enabled = true
	cfg.Tailscale.ExitNode = "100.90.3.4"
	for range 2 {
		if err := appendImportedTailscaleExitCandidates(&profile, cfg); err != nil {
			t.Fatal(err)
		}
	}
	for _, group := range profile.sections["proxy-groups"].Content {
		var decoded map[string]any
		if err := group.Decode(&decoded); err != nil {
			t.Fatal(err)
		}
		want := []string{"DIRECT"}
		switch decoded["name"] {
		case "User":
			want = append(want, config.TailscaleExitGroupName)
		case config.TailscaleExitGroupName:
			want = []string{config.TailscaleProxyName}
		}
		assertTailscaleGroupCandidates(t, decoded, want)
	}
}

func TestRenderManagedBaseProfileExcludesManagedRouting(t *testing.T) {
	cfg := config.Default()
	cfg.Tailscale.Enabled = true
	cfg.Tailscale.ExitNode = "100.90.3.4"
	cfg.Tailscale.AuthKeyFile = filepath.Join(t.TempDir(), "missing-key")
	for _, enabled := range []bool{false, true} {
		cfg.UpstreamProxy.Enabled = enabled
		cfg.UpstreamProxy.Name = "SampleProxy"
		cfg.UpstreamProxy.Type = "socks5"
		cfg.UpstreamProxy.Server = "127.0.0.1"
		cfg.UpstreamProxy.Port = 18080
		cfg.UpstreamProxy.MatchDomain = "example.com"
		base, err := RenderManagedBaseProfile(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := InspectImportedProfile([]byte(base)); err != nil {
			t.Fatalf("managed overlay base is not importable: %v\n%s", err, base)
		}
		for _, forbidden := range []string{config.TailscaleProxyName, LocalRoutingGroupPrefix, "device/"} {
			if strings.Contains(base, forbidden) {
				t.Fatalf("managed overlay base contains %s:\n%s", forbidden, base)
			}
		}
		if enabled {
			assertOrdered(t, base, "DOMAIN,example.com,open-surge-egress", "MATCH,DIRECT")
			if !strings.Contains(base, `name: "SampleProxy"`) || !strings.Contains(base, "name: open-surge-egress") {
				t.Fatalf("managed overlay base lost upstream proxy:\n%s", base)
			}
		} else if strings.Contains(base, "open-surge-egress") {
			t.Fatalf("disabled upstream generated an egress group:\n%s", base)
		}
	}
}

func TestTailscaleCandidateAliasExpansionRejectsCyclesAndExcessiveExpansion(t *testing.T) {
	cycle := &yaml.Node{Kind: yaml.AliasNode}
	cycle.Alias = cycle
	budget := 10
	if _, err := cloneTailscaleCandidateNode(cycle, map[*yaml.Node]bool{}, &budget, false); err == nil || !strings.Contains(err.Error(), "recursive") {
		t.Fatalf("recursive alias error = %v", err)
	}
	alias := &yaml.Node{Kind: yaml.AliasNode, Alias: sequenceNode(stringNode("DIRECT"), stringNode("REJECT"))}
	budget = 1
	if _, err := cloneTailscaleCandidateNode(alias, map[*yaml.Node]bool{}, &budget, false); err == nil || !strings.Contains(err.Error(), "node limit") {
		t.Fatalf("excessive alias expansion error = %v", err)
	}
}

func tailscaleSelectorTestConfig(t *testing.T, source string) config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = filepath.Join(dir, "profile.yaml")
	cfg.Tailscale.Enabled = true
	cfg.Tailscale.ExitNode = "100.90.3.4"
	cfg.Tailscale.AuthKeyFile = filepath.Join(dir, "tailscale-auth-key")
	cfg.Tailscale.StateDir = filepath.Join(dir, "tailscale-state")
	if err := os.WriteFile(cfg.Mihomo.Profile, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Tailscale.AuthKeyFile, []byte("tskey-auth-selector-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func renderedTailscaleSelectorGroups(t *testing.T, rendered string) map[string]map[string]any {
	t.Helper()
	var document struct {
		Groups []map[string]any `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &document); err != nil {
		t.Fatal(err)
	}
	groups := make(map[string]map[string]any, len(document.Groups))
	for _, group := range document.Groups {
		groups[group["name"].(string)] = group
	}
	return groups
}

func tailscaleGroupCandidates(t *testing.T, group map[string]any) []string {
	t.Helper()
	var candidates []string
	if group == nil {
		t.Fatal("missing proxy group")
	}
	if values, ok := group["proxies"].([]any); ok {
		for _, value := range values {
			candidates = append(candidates, value.(string))
		}
	}
	return candidates
}

func assertTailscaleGroupCandidates(t *testing.T, group map[string]any, want []string) {
	t.Helper()
	if got := tailscaleGroupCandidates(t, group); !reflect.DeepEqual(got, want) {
		t.Fatalf("group %v candidates = %#v, want %#v", group["name"], got, want)
	}
}

func TestTailscaleRulesReuseScopedLocalMacIPv6Identities(t *testing.T) {
	tests := []struct {
		name       string
		dnsIPv6    bool
		tunIPv6    string
		wantIPv6   string
		rejectIPv6 string
	}{
		{
			name:       "fake AAAA system TUN",
			dnsIPv6:    true,
			tunIPv6:    config.TUNIPv6Off,
			wantIPv6:   localRoutingHostTUNIPv6Source(),
			rejectIPv6: localRoutingFakeIPv6Source(),
		},
		{
			name:       "effective host IPv6 system TUN",
			dnsIPv6:    true,
			tunIPv6:    config.TUNIPv6Always,
			wantIPv6:   localRoutingHostTUNIPv6Source(),
			rejectIPv6: localRoutingFakeIPv6Source(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Transparent.Mode = config.TransparentModeTUN
			cfg.Transparent.TUNIPv6 = tt.tunIPv6
			cfg.DNS.IPv6 = tt.dnsIPv6
			cfg.Tailscale.Enabled = true
			cfg.Tailscale.AllowMac = true
			cfg.Tailscale.MagicDNSSuffixes = []string{"home.example.ts.net"}

			rules := strings.Join(renderTailscaleRules(cfg, policySections{}), "\n")
			want := "(IN-NAME," + SystemTUNListenerName + "),(SRC-IP-CIDR," + tt.wantIPv6 + ")"
			if !strings.Contains(rules, want) {
				t.Fatalf("Tailscale rules missing scoped local IPv6 identity %q:\n%s", want, rules)
			}
			for _, forbidden := range []string{
				"SRC-IP-CIDR," + tt.rejectIPv6,
				"SRC-IP-CIDR," + config.MihomoFakeIPv6Range,
				"SRC-IP-CIDR," + config.DownstreamIPv6Prefix,
				"IN-NAME," + config.IPv6PacketListenerName,
			} {
				if strings.Contains(rules, forbidden) {
					t.Fatalf("Tailscale local identity crossed IPv6 boundary with %q:\n%s", forbidden, rules)
				}
			}
		})
	}
}

func TestTailscaleTargetsRejectUnauthorizedSourcesBeforeDirectRules(t *testing.T) {
	cfg := config.Default()
	cfg.Tailscale.Enabled = true
	cfg.Tailscale.AllowMac = false
	cfg.Tailscale.MagicDNSSuffixes = []string{"lab.example.ts.net"}
	cfg.Tailscale.PeerCIDRs = []string{"100.82.10.7/32", "fd7a:115c:a1e0::7/128"}
	cfg.Tailscale.SubnetRoutes = []string{"10.203.77.0/24"}

	rules := renderTailscaleRules(cfg, policySections{})
	want := []string{
		"DOMAIN-SUFFIX,lab.example.ts.net,REJECT",
		"IP-CIDR,100.82.10.7/32,REJECT",
		"IP-CIDR6,fd7a:115c:a1e0::7/128,REJECT",
		"IP-CIDR,10.203.77.0/24,REJECT",
	}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("renderTailscaleRules() = %#v, want %#v", rules, want)
	}
}

func TestTailscaleRouteAddressesPreserveDefaultTUNCapture(t *testing.T) {
	cfg := config.Default()
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Tailscale.Enabled = true
	cfg.Tailscale.PeerCIDRs = []string{"100.82.10.7/32"}
	cfg.Tailscale.SubnetRoutes = []string{"10.203.77.0/24"}

	got := renderTUNRouteAddresses(cfg, "192.168.48.0/22")
	assertRenderedRouteCoverage(t, got, "1.1.1.1", true)
	assertRenderedRouteCoverage(t, got, "198.18.0.4", true)
	assertRenderedRouteCoverage(t, got, "10.1.2.3", false)
	assertRenderedRouteCoverage(t, got, "172.16.2.3", false)
	assertRenderedRouteCoverage(t, got, "192.168.1.2", false)
	assertRenderedRouteCoverage(t, got, "192.168.50.2", false)
	for _, expected := range []string{"    - 100.82.10.7/32", "    - 10.203.77.0/24"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("route-address output missing %q:\n%s", expected, got)
		}
	}
	if strings.Count(got, "100.82.10.7/32") != 1 {
		t.Fatalf("exact peer route must remain distinct:\n%s", got)
	}

	cfg.Transparent.TUNIPv6 = config.TUNIPv6Always
	got = renderTUNRouteAddresses(cfg, "192.168.48.0/22")
	assertRenderedRouteCoverage(t, got, "2606:4700:4700::1111", true)
	assertRenderedRouteCoverage(t, got, "::1", false)
}

func TestMacIPv6RoutesDoNotDependOnDownstreamOrTailnet(t *testing.T) {
	for _, peers := range [][]string{nil, {"100.82.10.7/32"}, {"fd7a:115c:a1e0::7/128"}} {
		for _, downstream := range []string{config.TUNIPv6Off, config.TUNIPv6Always} {
			for _, aaaa := range []bool{false, true} {
				cfg := config.Default()
				cfg.Transparent.Mode = config.TransparentModeTUN
				cfg.Transparent.TUNIPv6 = downstream
				cfg.DNS.IPv6 = aaaa
				cfg.Tailscale.Enabled = len(peers) > 0
				cfg.Tailscale.PeerCIDRs = peers
				got := renderTUNRouteAddresses(cfg, "192.168.1.0/24")
				for _, addr := range []string{"114.114.114.114", "198.18.1.2", "2001:4860:4860::8888", "fdfe:dcba:9876::ffff:ffff:ffff:ffff"} {
					assertRenderedRouteCoverage(t, got, addr, true)
				}
				for _, addr := range []string{"192.168.1.1", "fe80::1", "ff02::1", "::1", "fdfe:dcba:9878::7", "fd00::1"} {
					assertRenderedRouteCoverage(t, got, addr, false)
				}
				for _, peer := range peers {
					if strings.Count(got, "    - "+peer+"\n") != 1 && !strings.HasSuffix(got, "    - "+peer) {
						t.Fatalf("missing distinct peer route %s: %s", peer, got)
					}
				}
			}
		}
	}
}

func assertRenderedRouteCoverage(t *testing.T, rendered, address string, want bool) {
	t.Helper()
	ip := netip.MustParseAddr(address)
	matched := false
	for _, line := range strings.Split(rendered, "\n") {
		value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
		prefix, err := netip.ParsePrefix(value)
		if err == nil && prefix.Contains(ip) {
			matched = true
			break
		}
	}
	if matched != want {
		t.Fatalf("route coverage for %s = %t, want %t:\n%s", address, matched, want, rendered)
	}
}
