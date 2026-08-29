package mihomo

import (
	"net/netip"
	"reflect"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

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
			wantIPv6:   localRoutingFakeIPv6Source(),
			rejectIPv6: localRoutingHostTUNIPv6Source(),
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
			want := "(IN-NAME," + localRoutingSystemTUNName + "),(SRC-IP-CIDR," + tt.wantIPv6 + ")"
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

	got := renderTailscaleRouteAddresses(cfg, "192.168.48.0/22")
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
	got = renderTailscaleRouteAddresses(cfg, "192.168.48.0/22")
	assertRenderedRouteCoverage(t, got, "2606:4700:4700::1111", true)
	assertRenderedRouteCoverage(t, got, "::1", false)
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
