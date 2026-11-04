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

func TestRenderSnifferGatewayOwnership(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, imported := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%t/imported=%t", enabled, imported), func(t *testing.T) {
				cfg := config.Default()
				cfg.Mihomo.SnifferEnabled = enabled
				if imported {
					cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
					cfg.Mihomo.Profile = filepath.Join(t.TempDir(), "profile.yaml")
					// Imported enablement, destination rewriting and port overrides
					// must not override the gateway's single switch and defaults.
					profile := fmt.Sprintf("sniffer:\n  enable: %t\n  override-destination: true\n  sniff: {TLS: {ports: [9443]}}\nrules: ['MATCH,DIRECT']\n", !enabled)
					if err := os.WriteFile(cfg.Mihomo.Profile, []byte(profile), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				rendered, err := RenderConfig(cfg)
				if err != nil {
					t.Fatal(err)
				}
				var root map[string]any
				if err := yaml.Unmarshal([]byte(rendered), &root); err != nil {
					t.Fatal(err)
				}
				want := map[string]any{"enable": enabled}
				if enabled {
					want["force-dns-mapping"] = true
					want["parse-pure-ip"] = true
					want["override-destination"] = false
					want["sniff"] = map[string]any{
						"HTTP": map[string]any{"ports": []any{80, "8080-8880"}, "override-destination": false},
						"TLS":  map[string]any{"ports": []any{443, 8443}},
						"QUIC": map[string]any{"ports": []any{443}},
					}
				}
				if !reflect.DeepEqual(root["sniffer"], want) {
					t.Fatalf("sniffer = %#v, want %#v", root["sniffer"], want)
				}
			})
		}
	}
}
