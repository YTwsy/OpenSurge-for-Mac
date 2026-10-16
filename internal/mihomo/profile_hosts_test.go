package mihomo

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
)

func TestImportedHostsSurviveCompositionAndPreparedRendering(t *testing.T) {
	for _, hosts := range []struct{ name, yaml string }{
		{"block", `hosts:
  node.example.com: edge.real.example.net
  '*.example.net': 203.0.113.10
  ipv6.example.com: '2001:db8::10'
  addresses.example.com: &addresses [203.0.113.11, '2001:db8::11']
  shared.example.com: *addresses
`},
		{"flow", `hosts: {node.example.com: edge.real.example.net, '*.example.net': 203.0.113.10, ipv6.example.com: '2001:db8::10', addresses.example.com: &addresses [203.0.113.11, '2001:db8::11'], shared.example.com: *addresses}
`},
	} {
		for _, overlayEnabled := range []bool{false, true} {
			for _, useHosts := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/overlay=%t/use-hosts=%t", hosts.name, overlayEnabled, useHosts), func(t *testing.T) {
					source := hosts.yaml + fmt.Sprintf(`dns:
  enable: true
  listen: 0.0.0.0:7874
  use-hosts: %t
  use-system-hosts: false
  nameserver: [192.0.2.53]
  proxy-server-nameserver: [udp://127.0.0.1:7874]
  fake-ip-filter: ['+.real.example.net']
proxies:
  - {name: Imported, type: http, server: node.example.com, port: 8080}
proxy-groups:
  - {name: Proxy, type: select, proxies: [Imported]}
rules: ['MATCH,Proxy']
`, useHosts)
					decode := func(body string) map[string]any {
						t.Helper()
						var document map[string]any
						if err := yaml.Unmarshal([]byte(body), &document); err != nil {
							t.Fatal(err)
						}
						return document
					}
					original := decode(source)
					assertHosts := func(stage, body string) map[string]any {
						t.Helper()
						document := decode(body)
						if !reflect.DeepEqual(document["hosts"], original["hosts"]) {
							t.Fatalf("%s changed hosts: got %#v, want %#v", stage, document["hosts"], original["hosts"])
						}
						return document
					}
					cfg := config.Default()
					cfg.LocalSystemDNS.Enabled = false
					cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
					cfg.Mihomo.Profile = filepath.Join(t.TempDir(), "profile.yaml")
					writeProfile := func(body string) {
						t.Helper()
						if err := os.WriteFile(cfg.Mihomo.Profile, []byte(body), 0o600); err != nil {
							t.Fatal(err)
						}
					}
					writeProfile(source)
					sections, err := LoadImportedProfileSections(cfg.Mihomo.Profile)
					if err != nil {
						t.Fatal(err)
					}
					assertHosts("import", sections)
					overlay := ProfileOverlayDocument{
						SchemaVersion: ProfileOverlaySchemaVersion,
						Enabled:       overlayEnabled,
						Rules:         ProfileOverlayRuleOps{Prepend: []string{"DOMAIN,local.example,DIRECT"}},
						DNS:           ProfileOverlayDNSOps{Append: map[string][]any{"fake-ip-filter": {"+.overlay.example"}}},
					}
					composition, err := ComposeProfileOverlay([]byte(source), overlay)
					if err != nil {
						t.Fatal(err)
					}
					assertHosts("overlay", composition.ProfileYAML)
					writeProfile(composition.ProfileYAML)
					final, err := RenderConfig(cfg)
					if err != nil {
						t.Fatal(err)
					}
					prepared, err := RenderPreparedConfig(final, "127.0.0.1:19090", "test-secret")
					if err != nil {
						t.Fatal(err)
					}
					for _, stage := range []struct{ name, body, listen string }{
						{"gateway", final, "0.0.0.0:1053"},
						{"prepared", prepared, ""},
					} {
						document := assertHosts(stage.name, stage.body)
						dns := document["dns"].(map[string]any)
						if dns["listen"] != stage.listen || dns["use-hosts"] != useHosts {
							t.Fatalf("%s changed DNS ownership or use-hosts: %#v", stage.name, dns)
						}
						for _, field := range []string{"nameserver", "proxy-server-nameserver", "use-system-hosts"} {
							if !reflect.DeepEqual(dns[field], original["dns"].(map[string]any)[field]) {
								t.Fatalf("%s changed dns.%s", stage.name, field)
							}
						}
						filters := []any{"+.real.example.net"}
						if overlayEnabled {
							filters = append(filters, "+.overlay.example")
						}
						if !reflect.DeepEqual(dns["fake-ip-filter"], filters) || !reflect.DeepEqual(document["proxies"], original["proxies"]) {
							t.Fatalf("%s changed filters or node definitions", stage.name)
						}
					}
					persisted, err := os.ReadFile(cfg.Mihomo.Profile)
					if err != nil || string(persisted) != composition.ProfileYAML {
						t.Fatalf("rendering mutated imported profile: %v", err)
					}
				})
			}
		}
	}
}
