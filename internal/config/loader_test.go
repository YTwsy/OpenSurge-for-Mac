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
	if cfg.Mihomo.MixedPort != 7890 {
		t.Fatalf("Mihomo.MixedPort = %d", cfg.Mihomo.MixedPort)
	}
	if cfg.Mihomo.RedirPort != 0 {
		t.Fatalf("Mihomo.RedirPort = %d", cfg.Mihomo.RedirPort)
	}
	if cfg.DHCP.Binary != "dnsmasq" {
		t.Fatalf("DHCP.Binary = %q", cfg.DHCP.Binary)
	}
}
