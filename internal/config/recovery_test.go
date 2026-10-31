package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoveryLoadDoesNotValidateFutureDeployment(t *testing.T) {
	cfg := Default()
	cfg.Runtime.Dir = filepath.Join(t.TempDir(), "runtime")
	cfg.Gateway.LANIP = "192.0.2.10" // Existing DHCP pool is on another LAN.
	cfg.DevicePolicy.File = "missing-policy.json"
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("new deployments must reject the invalid desired configuration")
	}
	if _, err := LoadRuntime(path); err == nil {
		t.Fatal("runtime deployment validation unexpectedly accepted the invalid pool")
	}
	got, err := LoadRecovery(path)
	if err != nil || got.Gateway.LANIP != cfg.Gateway.LANIP || got.Runtime.Dir != cfg.Runtime.Dir {
		t.Fatalf("recovery lost persisted values: cfg=%+v err=%v", got, err)
	}
	if got.DevicePolicy.File != filepath.Join(filepath.Dir(path), "missing-policy.json") {
		t.Fatalf("relative policy path = %q", got.DevicePolicy.File)
	}
}

func TestRecoveryLoadRejectsUnknownPathsAndMalformedInput(t *testing.T) {
	for _, content := range []string{
		"runtime:\n  dir: \"\"\n",
		"mihomo:\n  config: \"\"\n",
		"dns:\n  port: invalid\n",
		"gateway:\n  typo: en0\n",
		"broken line\n",
	} {
		t.Run(strings.ReplaceAll(content, "\n", "_"), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadRecovery(path); err == nil {
				t.Fatal("recovery must not invent paths or ignore malformed configuration")
			}
		})
	}
}
