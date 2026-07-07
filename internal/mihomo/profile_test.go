package mihomo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
