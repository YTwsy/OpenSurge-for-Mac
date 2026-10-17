package config

import (
	"net/netip"
	"testing"
)

// This matrix deliberately does not inspect host interfaces. In particular,
// an existing IPv6 TUN must not hide a cold-start failure without native IPv6.
func TestOpenSurgeIPv6WithoutNativeUplink(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		engine, tun, host, dns, downstream bool
		keep                               bool
	}{
		{"host TUN without AAAA", true, true, true, false, false, true},
		{"host TUN with AAAA", true, true, true, true, false, true},
		{"host opt-out", true, true, false, false, false, false},
		{"host opt-out with AAAA", true, true, false, true, false, false},
		{"host opt-out with downstream", true, true, false, true, true, false},
		{"downstream without AAAA", true, true, false, false, true, false},
		{"engine disabled", false, true, true, true, true, false},
		{"inactive TUN without other IPv6", true, false, true, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := &RawConfig{IPv6: tc.engine}
			raw.Tun.Enable = tc.tun
			raw.Tun.Inet6Address = []netip.Prefix{}
			if tc.host {
				raw.Tun.Inet6Address = []netip.Prefix{netip.MustParsePrefix("fdfe:dcba:9877::1/126")}
			}
			raw.DNS.IPv6 = tc.dns
			raw.DNS.FakeIPRange6 = "fdfe:dcba:9876::/64"
			if tc.downstream {
				raw.Listeners = []map[string]any{{"type": "opensurge-packet"}}
			}
			parseOpenSurgeIPv6(raw, func() bool { return false })
			if got := len(raw.Tun.Inet6Address) != 0; got != tc.keep {
				t.Fatalf("host IPv6 retained=%v, want %v", got, tc.keep)
			}
			if tc.engine && (tc.dns || tc.downstream) && raw.DNS.FakeIPRange6 == "" {
				t.Fatal("independent DNS/downstream IPv6 cleared")
			}
			if raw.DNS.IPv6 != tc.dns || (len(raw.Listeners) > 0) != tc.downstream {
				t.Fatal("independent preferences changed")
			}
		})
	}
}
