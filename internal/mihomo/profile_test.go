package mihomo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadImportedProfileSectionsKeepsOnlyProxyAndRuleSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := `mixed-port: 7890
allow-lan: false
dns:
  enable: false
proxies:
  - name: Imported
    type: http
    server: 203.0.113.10
    port: 8080
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
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	sections, err := LoadImportedProfileSections(path)
	if err != nil {
		t.Fatalf("LoadImportedProfileSections() error = %v", err)
	}
	for _, want := range []string{
		"proxies:",
		"proxy-groups:",
		"rules:",
		"- DOMAIN-SUFFIX,example.com,Proxy",
	} {
		if !strings.Contains(sections, want) {
			t.Fatalf("imported sections missing %q:\n%s", want, sections)
		}
	}
	for _, notWant := range []string{
		"mixed-port:",
		"allow-lan:",
		"dns:",
		"tun:",
	} {
		if strings.Contains(sections, notWant) {
			t.Fatalf("imported sections kept gateway-owned %q:\n%s", notWant, sections)
		}
	}
}

func TestLoadImportedProfileSectionsRewritesProviderPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "providers"), 0o755); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(dir, "profile.yaml")
	body := `proxy-providers:
  local:
    type: file
    path: ./providers/proxies.yaml # keep comment
  quoted:
    type: file
    path: "./providers/quoted.yaml"
  absolute:
    type: file
    path: /var/tmp/proxies.yaml
rule-providers:
  cn:
    type: file
    path: './providers/cn.yaml'
  remote:
    type: http
    path: https://example.com/rules.yaml
rules:
  - RULE-SET,cn,DIRECT
  - MATCH,DIRECT
`
	if err := os.WriteFile(profilePath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	sections, err := LoadImportedProfileSections(profilePath)
	if err != nil {
		t.Fatalf("LoadImportedProfileSections() error = %v", err)
	}

	for _, want := range []string{
		"path: " + filepath.Join(dir, "providers", "proxies.yaml") + " # keep comment",
		`path: "` + filepath.Join(dir, "providers", "quoted.yaml") + `"`,
		`path: '` + filepath.Join(dir, "providers", "cn.yaml") + `'`,
		"path: /var/tmp/proxies.yaml",
		"path: https://example.com/rules.yaml",
		"- RULE-SET,cn,DIRECT",
	} {
		if !strings.Contains(sections, want) {
			t.Fatalf("imported sections missing %q:\n%s", want, sections)
		}
	}
}

func TestLoadImportedProfileSectionsRequiresRules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	if err := os.WriteFile(path, []byte("proxies: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadImportedProfileSections(path)
	if err == nil {
		t.Fatalf("LoadImportedProfileSections() succeeded")
	}
	if !strings.Contains(err.Error(), "top-level rules section") {
		t.Fatalf("LoadImportedProfileSections() error = %q", err)
	}
}

func TestLoadImportedProfileSectionsSupportsFlowStyleRulesAndQuotedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := "'proxy-groups': [{name: Proxy, type: select, proxies: [DIRECT]}]\r\n" +
		"'rules': [\r\n" +
		"  'DOMAIN-SUFFIX,deeplx.org,DIRECT',\r\n" +
		"  \"DOMAIN-SUFFIX,derp.tailscale.com,DIRECT\",\r\n" +
		"  'MATCH,Proxy'\r\n" +
		"]\r\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	rendered, err := LoadImportedProfileSections(path)
	if err != nil {
		t.Fatalf("LoadImportedProfileSections() error = %v", err)
	}
	var decoded struct {
		ProxyGroups []struct {
			Name string `yaml:"name"`
		} `yaml:"proxy-groups"`
		Rules []string `yaml:"rules"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &decoded); err != nil {
		t.Fatalf("rendered profile is invalid YAML: %v\n%s", err, rendered)
	}
	if len(decoded.ProxyGroups) != 1 || decoded.ProxyGroups[0].Name != "Proxy" {
		t.Fatalf("proxy groups = %#v", decoded.ProxyGroups)
	}
	wantRules := []string{
		"DOMAIN-SUFFIX,deeplx.org,DIRECT",
		"DOMAIN-SUFFIX,derp.tailscale.com,DIRECT",
		"MATCH,Proxy",
	}
	if strings.Join(decoded.Rules, "\n") != strings.Join(wantRules, "\n") {
		t.Fatalf("rules = %#v, want %#v", decoded.Rules, wantRules)
	}
}

func TestLoadImportedProfileSectionsSupportsEmptyFlowRules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	if err := os.WriteFile(path, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rendered, err := LoadImportedProfileSections(path)
	if err != nil {
		t.Fatalf("LoadImportedProfileSections() error = %v", err)
	}
	var decoded struct {
		Rules []string `yaml:"rules"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &decoded); err != nil {
		t.Fatalf("rendered profile is invalid YAML: %v\n%s", err, rendered)
	}
	if decoded.Rules == nil || len(decoded.Rules) != 0 {
		t.Fatalf("rules = %#v, want an empty sequence", decoded.Rules)
	}
}

func TestLoadImportedProfileSectionsRewritesFlowProviderPaths(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	body := `proxy-providers: {local: {type: file, path: './providers/a#b.yaml'}, absolute: {type: file, path: /var/tmp/proxies.yaml}}
rule-providers: {remote: {type: http, path: "https://example.com/rules.yaml"}}
rules: ['MATCH,DIRECT']
`
	if err := os.WriteFile(profilePath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	rendered, err := LoadImportedProfileSections(profilePath)
	if err != nil {
		t.Fatalf("LoadImportedProfileSections() error = %v", err)
	}
	var decoded struct {
		ProxyProviders map[string]struct {
			Path string `yaml:"path"`
		} `yaml:"proxy-providers"`
		RuleProviders map[string]struct {
			Path string `yaml:"path"`
		} `yaml:"rule-providers"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &decoded); err != nil {
		t.Fatalf("rendered profile is invalid YAML: %v\n%s", err, rendered)
	}
	if got, want := decoded.ProxyProviders["local"].Path, filepath.Join(dir, "providers", "a#b.yaml"); got != want {
		t.Fatalf("local provider path = %q, want %q", got, want)
	}
	if got := decoded.ProxyProviders["absolute"].Path; got != "/var/tmp/proxies.yaml" {
		t.Fatalf("absolute provider path = %q", got)
	}
	if got := decoded.RuleProviders["remote"].Path; got != "https://example.com/rules.yaml" {
		t.Fatalf("remote provider path = %q", got)
	}
}

func TestInspectImportedProfileRejectsMultipleDocumentsAndDanglingAliases(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "multiple documents",
			body: "rules: ['MATCH,DIRECT']\n---\nrules: ['MATCH,DIRECT']\n",
			want: "multiple YAML documents",
		},
		{
			name: "alias from excluded section",
			body: "shared: &shared [MATCH,DIRECT]\nrules: *shared\n",
			want: `references YAML anchor "shared" outside importable sections`,
		},
		{
			name: "non-string rule",
			body: "rules: [true]\n",
			want: "rules entries must be strings",
		},
		{
			name: "duplicate top-level section",
			body: "rules: ['MATCH,DIRECT']\nrules: ['MATCH,Proxy']\n",
			want: `duplicate top-level section "rules"`,
		},
		{
			name: "duplicate nested mapping key",
			body: "proxy-groups: [{name: Main, name: Backup, type: select, proxies: [DIRECT]}]\nrules: ['MATCH,DIRECT']\n",
			want: `duplicate mapping key "name"`,
		},
		{
			name: "wrong proxy section type",
			body: "proxies: {}\nrules: ['MATCH,DIRECT']\n",
			want: "proxies must be a sequence",
		},
		{
			name: "wrong provider section type",
			body: "rule-providers: []\nrules: ['MATCH,DIRECT']\n",
			want: "rule-providers must be a mapping",
		},
		{
			name: "provider entry is not a mapping",
			body: "rule-providers: {broken: true}\nrules: ['MATCH,DIRECT']\n",
			want: `rule-providers entry "broken" must be a mapping`,
		},
		{
			name: "provider path is not a string",
			body: "rule-providers: {broken: {type: file, path: 42}}\nrules: ['MATCH,DIRECT']\n",
			want: "provider path must be a string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := InspectImportedProfile([]byte(tt.body)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("InspectImportedProfile() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadImportedProfileSectionsPreservesAnchorDefinitionOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := `rule-providers:
  shared: &shared
    type: inline
    behavior: domain
    payload: [example.com]
proxy-providers:
  mirrored: *shared
rules: ['MATCH,DIRECT']
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	rendered, err := LoadImportedProfileSections(path)
	if err != nil {
		t.Fatalf("LoadImportedProfileSections() error = %v", err)
	}
	if strings.Index(rendered, "rule-providers:") > strings.Index(rendered, "proxy-providers:") {
		t.Fatalf("rendered sections moved alias before its anchor:\n%s", rendered)
	}
	var decoded yaml.Node
	if err := yaml.Unmarshal([]byte(rendered), &decoded); err != nil {
		t.Fatalf("rendered retained-section alias is invalid: %v\n%s", err, rendered)
	}
}

func TestInspectImportedProfileUsesSemanticTerminalMatchValidation(t *testing.T) {
	inspection, err := InspectImportedProfile([]byte("rules: ['DOMAIN,example.com,DIRECT', MATCH]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.TerminalMatch || inspection.RuleCount != 2 || len(inspection.Warnings) != 0 {
		t.Fatalf("inspection = %#v", inspection)
	}

	for _, body := range []string{
		"rules: ['MATCH,DIRECT', 'MATCH,Proxy']\n",
		"rules: ['MATCH,DIRECT', 'DOMAIN,example.com,DIRECT']\n",
	} {
		if _, err := InspectImportedProfile([]byte(body)); err == nil {
			t.Fatalf("InspectImportedProfile(%q) succeeded", body)
		}
	}
}
