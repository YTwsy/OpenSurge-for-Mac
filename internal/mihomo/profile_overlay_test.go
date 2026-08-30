package mihomo

import (
	"strings"
	"testing"
)

func TestComposeProfileOverlayAppliesExplicitOperations(t *testing.T) {
	source := []byte(`proxies:
  - name: Existing
    type: http
    server: old.example.com
    port: 8080
proxy-providers:
  Airport:
    type: http
    url: https://example.com/proxies.yaml
    path: ./providers/airport.yaml
proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - Existing
    use:
      - Airport
rule-providers:
  existing-rules:
    type: http
    behavior: domain
    url: https://example.com/existing.yaml
    path: ./rules/existing.yaml
rules:
  - DOMAIN,source.example,Proxy
  - MATCH,DIRECT
dns:
  nameserver:
    - 1.1.1.1
  fake-ip-filter:
    - '*.lan'
`)
	overlay := ProfileOverlayDocument{
		SchemaVersion: ProfileOverlaySchemaVersion,
		Enabled:       true,
		Rules: ProfileOverlayRuleOps{
			Prepend:           []string{"DOMAIN,first.example,DIRECT"},
			AppendBeforeMatch: []string{"RULE-SET,extra-rules,Proxy"},
		},
		Proxies: ProfileOverlaySequenceOps{
			Add:     []map[string]any{{"name": "LAN-Proxy", "type": "socks5", "server": "192.168.1.10", "port": 1080}},
			Replace: []map[string]any{{"name": "Existing", "type": "http", "server": "new.example.com", "port": 8081}},
		},
		ProxyProviders: ProfileOverlayMappingOps{
			Add: map[string]map[string]any{"Backup": {"type": "http", "url": "https://example.com/backup.yaml", "path": "./providers/backup.yaml"}},
		},
		ProxyGroups: ProfileOverlayGroupOps{
			Add:   []map[string]any{{"name": "Manual", "type": "select", "proxies": []any{"LAN-Proxy", "DIRECT"}}},
			Patch: []ProfileOverlayGroupPatch{{Name: "Proxy", AppendProxies: []string{"LAN-Proxy"}, AppendUse: []string{"Backup"}}},
		},
		RuleProviders: ProfileOverlayMappingOps{
			Add: map[string]map[string]any{"extra-rules": {"type": "http", "behavior": "domain", "url": "https://example.com/extra.yaml", "path": "./rules/extra.yaml"}},
		},
		DNS: ProfileOverlayDNSOps{
			Merge:  map[string]any{"nameserver-policy": map[string]any{"geosite:private": "system"}},
			Append: map[string][]any{"fake-ip-filter": {"+.internal.example"}},
		},
	}

	composition, err := ComposeProfileOverlay(source, overlay)
	if err != nil {
		t.Fatalf("ComposeProfileOverlay() error = %v", err)
	}
	for _, want := range []string{
		"server: new.example.com",
		"name: LAN-Proxy",
		"Backup:",
		"name: Manual",
		`- "LAN-Proxy"`,
		"extra-rules:",
		"DOMAIN,first.example,DIRECT",
		"RULE-SET,extra-rules,Proxy",
		"nameserver-policy:",
		"+.internal.example",
	} {
		if !strings.Contains(composition.ProfileYAML, want) {
			t.Fatalf("composed profile missing %q:\n%s", want, composition.ProfileYAML)
		}
	}
	first := strings.Index(composition.ProfileYAML, "DOMAIN,first.example,DIRECT")
	sourceRule := strings.Index(composition.ProfileYAML, "DOMAIN,source.example,Proxy")
	appendRule := strings.Index(composition.ProfileYAML, "RULE-SET,extra-rules,Proxy")
	match := strings.Index(composition.ProfileYAML, "MATCH,DIRECT")
	if first < 0 || !(first < sourceRule && sourceRule < appendRule && appendRule < match) {
		t.Fatalf("unexpected rule order:\n%s", composition.ProfileYAML)
	}
	if composition.Inspection.RuleCount != 4 || !composition.Inspection.TerminalMatch {
		t.Fatalf("inspection = %#v", composition.Inspection)
	}
	if composition.Digest == "" {
		t.Fatal("composition digest is empty")
	}
}

func TestComposeProfileOverlayRejectsImplicitConflicts(t *testing.T) {
	source := []byte("proxies:\n  - name: Existing\n    type: direct\nproxy-groups:\n  - name: Proxy\n    type: select\n    proxies: [Existing]\nrules: ['MATCH,DIRECT']\n")
	tests := []struct {
		name    string
		doc     ProfileOverlayDocument
		wantErr string
	}{
		{
			name: "add existing target",
			doc: ProfileOverlayDocument{SchemaVersion: 1, Enabled: true, Proxies: ProfileOverlaySequenceOps{
				Add: []map[string]any{{"name": "Existing", "type": "direct"}},
			}},
			wantErr: "conflicts with imported proxies",
		},
		{
			name: "replace missing target",
			doc: ProfileOverlayDocument{SchemaVersion: 1, Enabled: true, Proxies: ProfileOverlaySequenceOps{
				Replace: []map[string]any{{"name": "Missing", "type": "direct"}},
			}},
			wantErr: "does not exist",
		},
		{
			name: "patch missing group",
			doc: ProfileOverlayDocument{SchemaVersion: 1, Enabled: true, ProxyGroups: ProfileOverlayGroupOps{
				Patch: []ProfileOverlayGroupPatch{{Name: "Missing", AppendProxies: []string{"DIRECT"}}},
			}},
			wantErr: "does not exist",
		},
		{
			name: "patch unknown proxy",
			doc: ProfileOverlayDocument{SchemaVersion: 1, Enabled: true, ProxyGroups: ProfileOverlayGroupOps{
				Patch: []ProfileOverlayGroupPatch{{Name: "Proxy", AppendProxies: []string{"Unknown"}}},
			}},
			wantErr: "unknown proxy or group",
		},
		{
			name: "new group with dangling proxy",
			doc: ProfileOverlayDocument{SchemaVersion: 1, Enabled: true, ProxyGroups: ProfileOverlayGroupOps{
				Add: []map[string]any{{"name": "Manual", "type": "select", "proxies": []any{"Unknown"}}},
			}},
			wantErr: "references unknown proxy or group",
		},
		{
			name: "rule with dangling policy",
			doc: ProfileOverlayDocument{SchemaVersion: 1, Enabled: true, Rules: ProfileOverlayRuleOps{
				Prepend: []string{"DOMAIN,example.com,Unknown"},
			}},
			wantErr: "unknown proxy or group",
		},
		{
			name: "rule with dangling provider",
			doc: ProfileOverlayDocument{SchemaVersion: 1, Enabled: true, Rules: ProfileOverlayRuleOps{
				Prepend: []string{"RULE-SET,missing,Proxy"},
			}},
			wantErr: "unknown rule provider",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ComposeProfileOverlay(source, tt.doc)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ComposeProfileOverlay() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseProfileOverlayRejectsGatewayOwnedAndAmbiguousFields(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "gateway DNS", body: "schema-version: 1\nenabled: true\ndns:\n  merge:\n    listen: 127.0.0.1:53\n", wantErr: "managed by OpenSurge"},
		{name: "terminal match", body: "schema-version: 1\nenabled: true\nrules:\n  prepend: [MATCH,DIRECT]\n", wantErr: "must not add MATCH"},
		{name: "unknown top-level", body: "schema-version: 1\nenabled: true\ntun:\n  enable: false\n", wantErr: "field tun not found"},
		{name: "reserved target", body: "schema-version: 1\nenabled: true\nproxy-groups:\n  add:\n    - name: open-surge/mac-user\n      type: select\n", wantErr: "reserved OpenSurge namespace"},
		{name: "managed Tailscale target", body: "schema-version: 1\nenabled: true\nproxies:\n  add:\n    - name: open-surge/tailscale\n      type: direct\n", wantErr: "reserved OpenSurge namespace"},
		{name: "managed Tailscale Exit group", body: "schema-version: 1\nenabled: true\nproxy-groups:\n  add:\n    - name: open-surge/tailscale-exit\n      type: select\n      proxies: [DIRECT]\n", wantErr: "reserved OpenSurge namespace"},
		{name: "unsupported DNS field", body: "schema-version: 1\nenabled: true\ndns:\n  merge:\n    arbitrary-option: true\n", wantErr: "not a supported resolver or filtering field"},
		{name: "untrimmed patch target", body: "schema-version: 1\nenabled: true\nproxy-groups:\n  patch:\n    - name: ' Proxy '\n      append-proxies: [DIRECT]\n", wantErr: "trimmed string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseProfileOverlay([]byte(tt.body))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ParseProfileOverlay() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestProfileOverlayDocumentRoundTrip(t *testing.T) {
	document := DefaultProfileOverlayDocument()
	document.Enabled = true
	document.Rules.Prepend = []string{"DOMAIN,example.com,DIRECT"}
	document.ProxyGroups.Patch = []ProfileOverlayGroupPatch{{Name: "Proxy", AppendProxies: []string{"DIRECT"}}}

	rendered, err := RenderProfileOverlay(document)
	if err != nil {
		t.Fatalf("RenderProfileOverlay() error = %v", err)
	}
	parsed, err := ParseProfileOverlay(rendered)
	if err != nil {
		t.Fatalf("ParseProfileOverlay() error = %v\n%s", err, rendered)
	}
	if !parsed.Enabled || len(parsed.Rules.Prepend) != 1 || len(parsed.ProxyGroups.Patch) != 1 {
		t.Fatalf("round trip document = %#v", parsed)
	}
}

func TestComposeProfileOverlayTailRulesRequireTerminalMatch(t *testing.T) {
	document := DefaultProfileOverlayDocument()
	document.Enabled = true
	document.Rules.AppendBeforeMatch = []string{"DOMAIN,example.com,DIRECT"}

	_, err := ComposeProfileOverlay([]byte("rules:\n  - DOMAIN,source.example,DIRECT\n"), document)
	if err == nil || !strings.Contains(err.Error(), "requires the imported profile to end with MATCH") {
		t.Fatalf("ComposeProfileOverlay() error = %v", err)
	}
}
