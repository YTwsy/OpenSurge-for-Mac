package mihomo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
)

func TestExternalDNSFilterComposition(t *testing.T) {
	cfg := config.Default()
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.ExternalDNSFilter = config.ExternalDNSFilterConfig{Enabled: true, Server: "127.0.0.1:5353", TimeoutMS: 2000, BypassDomains: []string{"private.example"}}
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = filepath.Join(t.TempDir(), "source.yaml")
	if err := os.WriteFile(cfg.Mihomo.Profile, []byte("dns:\n  opensurge-external-filter: {enabled: false, probe-name: bypass.example}\n  nameserver: [1.1.1.1]\nhosts: {fixed.example: 192.0.2.7}\nrules: ['MATCH,DIRECT']\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"opensurge-external-filter:", "server: 127.0.0.1:5353", "private.example", "- lan", "fixed.example"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %s: %s", want, rendered)
		}
	}
	if strings.Contains(rendered, "bypass.example") {
		t.Fatal("imported filter ownership escaped")
	}
	prepared, err := RenderPreparedConfig(rendered, "127.0.0.1:19090", "secret")
	if err != nil || !strings.Contains(prepared, "opensurge-external-filter:") {
		t.Fatal(prepared, err)
	}
	cfg.ExternalDNSFilter.Enabled = false
	rendered, err = RenderConfig(cfg)
	if err != nil || strings.Contains(rendered, "opensurge-external-filter") {
		t.Fatal("disabled config retained subscription filter", err)
	}
}

func TestExternalDNSFilterRequiresCapableEngine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("dns:\n  opensurge-external-filter: {enabled: true}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, validate := range []bool{true, false} {
		args, err := engineConfigArgs(filepath.Dir(path), path, validate)
		if err != nil || !strings.Contains(strings.Join(args, " "), "-require-opensurge-dns-filter") {
			t.Fatal(args, err)
		}
	}
}

func TestExternalDNSFilterPrivateDomainsAndDirectInterface(t *testing.T) {
	cfg := config.Default()
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Gateway.Interface = "bridge100"
	cfg.Gateway.UpstreamInterface = "en0"
	cfg.LocalSystemDNS.Domains = []string{"Corp.Example.", ""}
	cfg.Tailscale.Enabled = true
	cfg.Tailscale.MagicDNSSuffixes = []string{"tail.example"}
	cfg.ExternalDNSFilter = config.ExternalDNSFilterConfig{Enabled: true, TimeoutMS: 2000}
	for _, tc := range []struct{ server, iface string }{
		{"192.168.50.2:5353", "bridge100"},
		{"[::ffff:192.168.50.2]:5353", "bridge100"},
		{"203.0.113.7:53", "en0"},
		{"[2001:db8::7]:53", "en0"},
		{"127.0.0.1:5353", ""},
	} {
		cfg.ExternalDNSFilter.Server = tc.server
		rendered, err := renderExternalDNSFilter(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]map[string]any
		if err := yaml.Unmarshal([]byte(rendered), &fields); err != nil {
			t.Fatal(err)
		}
		if fields["opensurge-external-filter"]["interface"] != tc.iface {
			t.Fatal(tc, rendered)
		}
		for _, domain := range []string{"corp.example", "tail.example", "lan"} {
			if !strings.Contains(rendered, "- "+domain) {
				t.Fatal("missing private bypass", rendered)
			}
		}
	}
	cfg.Transparent.Mode = config.TransparentModeOff
	cfg.LocalSystemDNS.Domains = []string{"disabled.example"}
	rendered, err := renderExternalDNSFilter(cfg)
	if err != nil || strings.Contains(rendered, "disabled.example") {
		t.Fatal(rendered, err)
	}
}
