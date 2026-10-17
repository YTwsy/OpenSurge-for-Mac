package mihomo

import (
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func customDoc(yaml string) CustomDNSDocument {
	return CustomDNSDocument{SchemaVersion: 1, Enabled: true, YAML: yaml}
}

func TestCustomDNSImportReplacement(t *testing.T) {
	original := customDoc("hosts: {old.example: 192.0.2.1}\ndns: {nameserver: [1.1.1.1]}\n")
	imported, sections, err := ImportCustomDNS(original, "hosts", []byte("\ufeff# test\r\n192.0.2.8 NAS.HOME.ARPA. other.home.arpa\r\n2001:db8::8 nas.home.arpa\n192.0.2.8 nas.home.arpa\n"))
	if err != nil {
		t.Fatal(err)
	}
	root, _ := parseCustomDNS([]byte(imported.YAML))
	hosts := customValue(root, "hosts")
	values := customValue(hosts, "nas.home.arpa")
	if len(sections) != 1 || sections[0] != "hosts" || customValue(hosts, "old.example") != nil || len(values.Content) != 2 || customValue(root, "dns") == nil {
		t.Fatalf("bad replacement: %s", imported.YAML)
	}
	replaced, _, err := ImportCustomDNS(imported, "yaml", []byte("dns: {use-system-hosts: false}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(replaced.YAML, "nameserver") || !strings.Contains(replaced.YAML, "nas.home.arpa") {
		t.Fatal(replaced.YAML)
	}
	again, _, err := ImportCustomDNS(replaced, "yaml", []byte(replaced.YAML))
	if err != nil || again.YAML != replaced.YAML {
		t.Fatalf("import not idempotent: %v", err)
	}
	for _, input := range []string{"address=/nas/192.0.2.8", "not-an-IP nas.home.arpa", "192.0.2.8 *.example", "192.0.2.8 good.example\ninvalid bad.example"} {
		result, _, err := ImportCustomDNS(original, "hosts", []byte(input))
		if err == nil || strings.Contains(result.YAML, "good.example") {
			t.Fatalf("non-atomic import %q: %+v %v", input, result, err)
		}
	}
}

func TestCustomDNSRejectsInvalidDocuments(t *testing.T) {
	for _, input := range []string{
		"dns: {listen: ':53'}", "tun: {enable: true}", "hosts: {foo.example: null}", "dns: {use-hosts: yes}",
		"hosts: {Foo.Example: 192.0.2.1, foo.example.: 192.0.2.2}", "dns: {nameserver-policy: {geosite:cn: [1.1.1.1]}}",
		"dns: {fallback-filter: {geosite: [cn]}}", "dns: {nameserver: !custom [1.1.1.1]}",
		"hosts: &hosts {foo.example: *hosts}", "dns: {}\n---\nhosts: {}", "hosts: {foo.example: [bar.example]}",
		"dns: {fake-ip-filter: ['RULE-SET,cn,real-ip']}", "dns: {nameserver-policy: {foo.example: [1.1.1.1], FOO.EXAMPLE.: [8.8.8.8]}}",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := NormalizeCustomDNS(customDoc(input)); err == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
	if _, err := NormalizeCustomDNS(customDoc("hosts: {a.example: &ip 192.0.2.1, b.example: *ip}")); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeCustomDNS(customDoc("#" + strings.Repeat("x", MaxCustomDNSSize))); err == nil {
		t.Fatal("accepted oversized file")
	}
	if _, _, err := ImportCustomDNS(DefaultCustomDNSDocument(), "hosts", []byte{0xff}); err == nil {
		t.Fatal("accepted non-UTF8")
	}
}

func TestCustomDNSCompositionAndModes(t *testing.T) {
	base := `hosts:
  nas.home.arpa: [192.0.2.1, 192.0.2.2]
  inherited.example: 192.0.2.9
dns:
  use-hosts: false
  nameserver: [1.1.1.1]
  nameserver-policy:
    '+.corp.example': [1.1.1.1, 8.8.8.8]
  fake-ip-filter: [base.example]
rules: [MATCH,DIRECT]
`
	// Flow-style rules need a single string entry.
	base = strings.ReplaceAll(base, "[MATCH,DIRECT]", "['MATCH,DIRECT']")
	result, err := ComposeCustomDNS([]byte(base), customDoc(`hosts: {nas.home.arpa: 192.0.2.3}
dns:
  nameserver-policy: {'+.corp.example': [10.20.0.53]}
  fake-ip-filter: [custom.example, base.example]
`))
	if err != nil {
		t.Fatal(err)
	}
	root, _ := decodeSingleYAMLMapping([]byte(result.ProfileYAML))
	dns := customValue(root, "dns")
	if customValue(dns, "use-hosts").Value != "true" || customValue(customValue(root, "hosts"), "nas.home.arpa").Value != "192.0.2.3" || len(customValue(dns, "fake-ip-filter").Content) != 2 {
		t.Fatal(result.ProfileYAML)
	}
	if value := customValue(customValue(dns, "nameserver-policy"), "+.corp.example"); len(value.Content) != 1 || value.Content[0].Value != "10.20.0.53" {
		t.Fatal(result.ProfileYAML)
	}
	if !strings.Contains(result.ProfileYAML, "inherited.example") || len(result.Changes) == 0 {
		t.Fatal(result.ProfileYAML)
	}
	for _, tc := range []struct {
		name, mode, patch, want string
		bad                     bool
	}{
		{"blacklist append", "blacklist", "fake-ip-filter: [custom.example]", "base.example\n  - custom.example", false},
		{"whitelist append", "whitelist", "fake-ip-filter: [custom.example]", "base.example\n  - custom.example", false},
		{"mode switch", "blacklist", "fake-ip-filter-mode: whitelist\n  fake-ip-filter: [custom.example]", "custom.example", false},
		{"missing replacement", "blacklist", "fake-ip-filter-mode: whitelist", "", true},
		{"rule prepend", "rule", "fake-ip-filter: ['DOMAIN,custom.example,real-ip']", "DOMAIN,custom.example,real-ip", false},
		{"rule MATCH shadow", "rule", "fake-ip-filter: ['MATCH,real-ip']", "MATCH,real-ip", false},
		{"bad rule", "rule", "fake-ip-filter: [custom.example]", "", true},
		{"bad list", "blacklist", "fake-ip-filter: ['MATCH,real-ip']", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			list := "base.example"
			if tc.mode == "rule" {
				list = "MATCH,fake-ip"
			}
			source := "dns:\n  fake-ip-filter-mode: " + tc.mode + "\n  fake-ip-filter:\n    - " + list + "\nrules: ['MATCH,DIRECT']\n"
			composed, e := ComposeCustomDNS([]byte(source), customDoc("dns:\n  "+tc.patch+"\n"))
			if tc.bad {
				if e == nil {
					t.Fatal("expected error")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			r, _ := decodeSingleYAMLMapping([]byte(composed.ProfileYAML))
			f := customValue(customValue(r, "dns"), "fake-ip-filter")
			if tc.mode == "rule" && f.Content[0].Value != strings.Split(tc.want, "\n")[0] {
				t.Fatal(composed.ProfileYAML)
			}
			if tc.name == "mode switch" && len(f.Content) != 1 {
				t.Fatal(composed.ProfileYAML)
			}
			if tc.name == "blacklist append" || tc.name == "whitelist append" {
				if len(f.Content) != 2 || f.Content[0].Value != "base.example" {
					t.Fatal(composed.ProfileYAML)
				}
			}
		})
	}
	if _, err = ComposeCustomDNS([]byte(base), customDoc("hosts: {nas.home.arpa: 192.0.2.3}\ndns: {use-hosts: FALSE}")); err == nil {
		t.Fatal("accepted disabled hosts")
	}
	disabled := customDoc("hosts: {nas.home.arpa: 192.0.2.3}")
	disabled.Enabled = false
	off, err := ComposeCustomDNS([]byte(base), disabled)
	if err != nil || off.ProfileYAML != base {
		t.Fatalf("disabled altered source: %v", err)
	}
}

func TestCustomDNSDualEntryGuards(t *testing.T) {
	cfg := config.Default()
	cfg.Transparent.Mode = "tun"
	cfg.Transparent.TUNAutoRoute = true
	cfg.LocalSystemDNS.Enabled = true
	cfg.DHCP.Enabled = true
	cfg.DHCP.Domain = "lan"
	base := []byte("hosts: {inherited.example: secret.lan}\nrules: ['MATCH,DIRECT']\n")
	for _, tc := range []struct {
		yaml string
		bad  bool
	}{
		{"hosts: {nas.home.arpa: 192.0.2.8}", false},
		{"hosts: {nas: 192.0.2.8}", true}, {"hosts: {'*.lan': 192.0.2.8}", true},
		{"hosts: {'+.*': 192.0.2.8}", true}, {"hosts: {nas.example: inherited.example}", true},
		{"dns: {nameserver-policy: {'+.lan': [10.0.0.53]}}", true},
		{"hosts: {nas.example: next.example, next.example: nas.example}", true},
	} {
		t.Run(tc.yaml, func(t *testing.T) {
			doc := customDoc(tc.yaml)
			composed, err := ComposeCustomDNS(base, doc)
			if err == nil {
				normal, e := NormalizeCustomDNS(doc)
				if e != nil {
					t.Fatal(e)
				}
				candidate := cfg
				candidate.Mihomo.ProfileCustomDNSDigest = CustomDNSDigest(normal)
				err = ValidateCustomDNSCandidate(candidate, []byte(composed.ProfileYAML))
			}
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v, bad=%v", err, tc.bad)
			}
		})
	}
	doc, _ := NormalizeCustomDNS(customDoc("hosts: {nas.home.arpa: 192.0.2.8}"))
	composed, err := ComposeCustomDNS(base, doc)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Mihomo.ProfileCustomDNSDigest = CustomDNSDigest(doc)
	for _, mutate := range []func(*config.Config){func(c *config.Config) { c.DNS.Upstream = "8.8.8.8" }, func(c *config.Config) { c.LocalSystemDNS.Enabled = false }, func(c *config.Config) { c.Transparent.TUNAutoRoute = false }, func(c *config.Config) { c.DHCP.Domain = "home.arpa" }, func(c *config.Config) { c.Mihomo.ProfileCustomDNSDigest = "bad" }} {
		changed := cfg
		mutate(&changed)
		if err := ValidateCustomDNSCandidate(changed, []byte(composed.ProfileYAML)); err == nil {
			t.Fatal("candidate guard bypassed")
		}
	}
}

func TestCustomDNSExactAliasTargetsAndForwardingLoop(t *testing.T) {
	base := []byte("hosts: {'+.example': target.example, target.example: 192.0.2.1}\nrules: ['MATCH,DIRECT']\n")
	doc := customDoc("hosts: {alias.home.arpa: target.example}")
	if _, err := ComposeCustomDNS(base, doc); err != nil {
		t.Fatalf("exact IP target must win over inherited wildcard alias: %v", err)
	}
	if customOverlapsDHCP("nas.example", "branch.nas.example") {
		t.Fatal("exact domain outside DHCP suffix was rejected")
	}
	cfg := config.Default()
	cfg.Mihomo.ProfileCustomDNSDigest = "enabled"
	for _, fields := range []string{"nameserver: [" + cfg.Gateway.LANIP + "]", "fallback: [tcp://[" + config.DownstreamIPv6Gateway + "]:53]"} {
		if err := validateCustomDNSForwardingLoops(cfg, fields); err == nil {
			t.Fatalf("accepted loop: %s", fields)
		}
	}
	if err := validateCustomDNSForwardingLoops(cfg, "nameserver: [10.20.0.53, 114.114.114.114]"); err != nil {
		t.Fatal(err)
	}
	cfg.Mihomo.ProfileCustomDNSDigest = ""
	if err := validateCustomDNSForwardingLoops(cfg, "nameserver: ["+cfg.Gateway.LANIP+"]"); err != nil {
		t.Fatal("new guard affected disabled legacy configuration")
	}
}

func TestCustomDNSWildcardAliasSpecificity(t *testing.T) {
	base := []byte("hosts: {'.example': target.example, '*.example': 192.0.2.1}\nrules: ['MATCH,DIRECT']\n")
	if _, err := ComposeCustomDNS(base, customDoc("hosts: {alias.home.arpa: node.example}")); err != nil {
		t.Fatalf("one-label wildcard must beat suffix alias: %v", err)
	}
	for _, pattern := range []string{"+.*", "+.*.example", ".*.example"} {
		if !customDomainMatches(pattern, "deep.node.example") {
			t.Fatalf("wildcard did not match: %s", pattern)
		}
	}
	if customDomainMatches("*.example", "deep.node.example") {
		t.Fatal("single-label wildcard matched multiple labels")
	}
	if _, err := ComposeCustomDNS([]byte("rules: ['MATCH,DIRECT']"), customDoc("hosts: {'+.*': target.example, alias.home.arpa: target.example}")); err == nil {
		t.Fatal("wildcard alias cycle was not detected")
	}
}
