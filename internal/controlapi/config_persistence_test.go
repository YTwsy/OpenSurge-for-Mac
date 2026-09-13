package controlapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

// A Web/API topology save rewrites the authoritative YAML through
// config.Render. QNAP's real upstream router is intentionally not exposed as an
// editable ControlConfig field, so the save path must carry the already-loaded
// value through unchanged. Losing it makes Host Takeover impossible after the
// next container or NAS restart.
func TestApplyControlConfigPreservesUpstreamGateway(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opensurge.yaml")
	cfg := config.Default()
	cfg.Gateway.UpstreamGateway = "192.168.2.1"
	cfg.Runtime.Dir = filepath.Join(dir, "runtime")
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	cfg.DHCP.Enabled = false
	if err := os.WriteFile(configPath, []byte(config.Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}

	input := controlConfigFrom(cfg, fileDigest(configPath))
	payload, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applyControlConfig(configPath, fileDigest(configPath), payload); err != nil {
		t.Fatalf("applyControlConfig() error = %v", err)
	}

	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load(saved config) error = %v", err)
	}
	if loaded.Gateway.UpstreamGateway != cfg.Gateway.UpstreamGateway {
		t.Fatalf("upstream gateway after API save = %q, want %q", loaded.Gateway.UpstreamGateway, cfg.Gateway.UpstreamGateway)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `upstream_gateway: "192.168.2.1"`) {
		t.Fatalf("saved config does not persist upstream_gateway:\n%s", data)
	}
}
