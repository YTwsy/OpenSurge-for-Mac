package config

import (
	"os"
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
	if cfg.Mihomo.ProfileMode != MihomoProfileModeManaged {
		t.Fatalf("Mihomo.ProfileMode = %q", cfg.Mihomo.ProfileMode)
	}
	if cfg.Mihomo.Profile != "" {
		t.Fatalf("Mihomo.Profile = %q", cfg.Mihomo.Profile)
	}
	if cfg.DHCP.Binary != "dnsmasq" {
		t.Fatalf("DHCP.Binary = %q", cfg.DHCP.Binary)
	}
	if cfg.DNS.Upstream != "" {
		t.Fatalf("DNS.Upstream = %q", cfg.DNS.Upstream)
	}
	if cfg.Transparent.Mode != TransparentModeOff {
		t.Fatalf("Transparent.Mode = %q", cfg.Transparent.Mode)
	}
	if cfg.Transparent.TUNDevice != "utun123" {
		t.Fatalf("Transparent.TUNDevice = %q", cfg.Transparent.TUNDevice)
	}
	if cfg.UpstreamProxy.Enabled {
		t.Fatalf("UpstreamProxy.Enabled = true")
	}
	if cfg.UpstreamProxy.MatchDomain != "example.com" {
		t.Fatalf("UpstreamProxy.MatchDomain = %q", cfg.UpstreamProxy.MatchDomain)
	}
}

func TestLoadImportedMihomoProfileConfig(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	if err := os.WriteFile(profilePath, []byte("rules:\n  - MATCH,DIRECT\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
mihomo:
  profile_mode: "imported"
  profile: "` + profilePath + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Mihomo.ProfileMode != MihomoProfileModeImported {
		t.Fatalf("Mihomo.ProfileMode = %q", cfg.Mihomo.ProfileMode)
	}
	if cfg.Mihomo.Profile != profilePath {
		t.Fatalf("Mihomo.Profile = %q", cfg.Mihomo.Profile)
	}
}

func TestLoadResolvesRelativeMihomoProfileFromConfigDir(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	if err := os.WriteFile(profilePath, []byte("rules:\n  - MATCH,DIRECT\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
mihomo:
  profile_mode: "imported"
  profile: "./profile.yaml"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Mihomo.Profile != profilePath {
		t.Fatalf("Mihomo.Profile = %q, want %q", cfg.Mihomo.Profile, profilePath)
	}
}

func TestLoadImportedProfileExampleConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "examples", "config.imported-profile.example.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Mihomo.ProfileMode != MihomoProfileModeImported {
		t.Fatalf("Mihomo.ProfileMode = %q", cfg.Mihomo.ProfileMode)
	}
	if cfg.Mihomo.Profile == "" {
		t.Fatalf("Mihomo.Profile is empty")
	}
	if filepath.Base(cfg.Mihomo.Profile) != "mihomo-profile.example.yaml" {
		t.Fatalf("Mihomo.Profile = %q", cfg.Mihomo.Profile)
	}
}
