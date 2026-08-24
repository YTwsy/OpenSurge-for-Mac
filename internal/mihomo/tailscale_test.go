package mihomo

import (
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
