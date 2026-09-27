package mihomo

import (
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestSystemDNSPreservesPrivateResolversWithoutPublicFallback(t *testing.T) {
	cfg := config.Default()
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.LocalSystemDNS.Resolvers = []string{"192.168.1.1", "fe80::1%en0"}
	cfg.LocalSystemDNS.Domains = []string{"corp.example"}
	fields := "nameserver: [https://dns.example/dns-query]\nproxy-server-nameserver: [1.1.1.1]\nnameserver-policy:\n  '+.existing.lan': 10.0.0.1\nfake-ip-filter: ['+.existing.lan']\n"
	got, err := coordinateDNSResolvers(cfg, fields)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"https://dns.example/dns-query", "proxy-server-nameserver", "+.existing.lan", "+.corp.example", "+.home.arpa", "udp://[fe80::1%25en0]:53"} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %s: %s", part, got)
		}
	}
	if strings.Contains(got, "fallback:") || strings.Contains(got, config.LocalSystemDNSServer) {
		t.Fatal("system capture endpoint/router became public fallback")
	}
	if _, err := coordinateDNSResolvers(cfg, "nameserver: [system]\n"); err == nil {
		t.Fatal("recursive system DNS accepted")
	}
	if _, err := coordinateDNSResolvers(cfg, "nameserver: [1.1.1.1]\nfake-ip-filter: [system]\nnameserver-policy: {system: 192.168.1.1}\n"); err != nil {
		t.Fatal("domain named system is not a resolver cycle", err)
	}
	cfg.LocalSystemDNS.Enabled = false
	if _, err := coordinateDNSResolvers(cfg, "nameserver: [system]\n"); err != nil {
		t.Fatal("opt-out must preserve imported resolver semantics", err)
	}
}
