package config

import (
	"path/filepath"
	"testing"
)

func TestLoadExampleConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "examples", "config.example.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Gateway.Interface != "en0" {
		t.Fatalf("Gateway.Interface = %q", cfg.Gateway.Interface)
	}
	if cfg.Mihomo.RedirPort != 7892 {
		t.Fatalf("Mihomo.RedirPort = %d", cfg.Mihomo.RedirPort)
	}
}
