package controlapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestControlConfigExternalDNSFilterAndLegacyClient(t *testing.T) {
	cfg := config.Default()
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	path := filepath.Join(cfg.Runtime.Dir, "config.yaml")
	if err := os.WriteFile(path, []byte(config.Render(cfg)), 0600); err != nil {
		t.Fatal(err)
	}
	input := controlConfigFrom(cfg, fileDigest(path))
	input.ExternalDNSFilter = &config.ExternalDNSFilterConfig{Enabled: true, Server: "127.0.0.1:5353", TimeoutMS: 2000, BypassDomains: []string{"private.example"}}
	data, _ := json.Marshal(input)
	if _, err := applyControlConfig(path, input.Revision, data); err != nil {
		t.Fatal(err)
	}
	saved, err := config.Load(path)
	if err != nil || !reflect.DeepEqual(saved.ExternalDNSFilter, *input.ExternalDNSFilter) {
		t.Fatal(saved.ExternalDNSFilter, err)
	}
	legacy := controlConfigFrom(saved, fileDigest(path))
	legacy.ExternalDNSFilter = nil
	data, _ = json.Marshal(legacy)
	if _, err := applyControlConfig(path, legacy.Revision, data); err != nil {
		t.Fatal(err)
	}
	saved, err = config.Load(path)
	if err != nil || !saved.ExternalDNSFilter.Enabled || saved.ExternalDNSFilter.Server != "127.0.0.1:5353" {
		t.Fatal(saved.ExternalDNSFilter, err)
	}
}
