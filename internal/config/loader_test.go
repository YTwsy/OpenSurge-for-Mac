package config

import (
	"os"
	"path/filepath"
	"strings"
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
	if cfg.Gateway.Mode != GatewayModeIsolatedLAN {
		t.Fatalf("Gateway.Mode = %q", cfg.Gateway.Mode)
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
	if !cfg.Mihomo.StoreFakeIP {
		t.Fatal("Mihomo.StoreFakeIP = false")
	}
	if cfg.Mihomo.SnifferEnabled {
		t.Fatal("sniffer must default to disabled")
	}
	if cfg.DHCP.Binary != "dnsmasq" {
		t.Fatalf("DHCP.Binary = %q", cfg.DHCP.Binary)
	}
	if cfg.DNS.Upstream != MihomoDNSUpstream {
		t.Fatalf("DNS.Upstream = %q", cfg.DNS.Upstream)
	}
	if cfg.Transparent.Mode != TransparentModeOff {
		t.Fatalf("Transparent.Mode = %q", cfg.Transparent.Mode)
	}
	if cfg.Transparent.TUNDevice != "utun123" {
		t.Fatalf("Transparent.TUNDevice = %q", cfg.Transparent.TUNDevice)
	}
	if cfg.LocalSystemProxy.Enabled {
		t.Fatalf("LocalSystemProxy.Enabled = true")
	}
	if cfg.UpstreamProxy.Enabled {
		t.Fatalf("UpstreamProxy.Enabled = true")
	}
	if cfg.UpstreamProxy.MatchDomain != "example.com" {
		t.Fatalf("UpstreamProxy.MatchDomain = %q", cfg.UpstreamProxy.MatchDomain)
	}
}

func TestLoadSnifferPreference(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		enabled    bool
		invalid    bool
	}{
		{name: "legacy omitted", body: "mihomo:\n  store_fake_ip: true\n"},
		{name: "disabled", body: "mihomo:\n  sniffer_enabled: false\n"},
		{name: "enabled", body: "mihomo:\n  sniffer_enabled: true\n", enabled: true},
		{name: "invalid", body: "mihomo:\n  sniffer_enabled: invalid\n", invalid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if tt.invalid {
				if err == nil || !strings.Contains(err.Error(), "mihomo.sniffer_enabled must be a boolean") {
					t.Fatalf("invalid sniffer value: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Mihomo.SnifferEnabled != tt.enabled {
				t.Fatalf("SnifferEnabled = %t, want %t", cfg.Mihomo.SnifferEnabled, tt.enabled)
			}
			if err := os.WriteFile(path, []byte(Render(cfg)), 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(path)
			if err != nil || loaded.Mihomo.SnifferEnabled != tt.enabled {
				t.Fatalf("sniffer round trip = %t, err = %v", loaded.Mihomo.SnifferEnabled, err)
			}
		})
	}
}

func TestLoadSameLANGatewayMode(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
gateway:
  mode: "same_lan"
  interface: "en0"
  upstream_interface: "en0"

dhcp:
  enabled: false

transparent:
  mode: "tun"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Gateway.Mode != GatewayModeSameLAN {
		t.Fatalf("Gateway.Mode = %q", cfg.Gateway.Mode)
	}
	if cfg.DHCP.Enabled {
		t.Fatalf("DHCP.Enabled = true")
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

func TestLoadResolvesAndValidatesRelativeDevicePolicyFile(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "devices.json")
	policy := `{
  "profiles":[{"id":"default","default_policies":["DIRECT"]}],
  "devices":[{"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"default"}]
}`
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
device_policy:
  file: "./devices.json"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DevicePolicy.File != policyPath {
		t.Fatalf("DevicePolicy.File = %q, want %q", cfg.DevicePolicy.File, policyPath)
	}
}

func TestLoadRejectsDeviceReservationThatConflictsWithProtectedIPv4(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "devices.json")
	policy := `{"profiles":[{"id":"default","default_policies":["DIRECT"]}],"devices":[{"id":"phone","mac":"aa:bb:cc:dd:ee:01","ipv4":"192.168.50.101","profile":"default"}]}`
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
device_policy:
  file: "./devices.json"
  protected_ipv4: "192.168.50.101,192.168.50.253"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(configPath); err == nil || !strings.Contains(err.Error(), "conflicts with a protected") {
		t.Fatalf("Load() error = %v", err)
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

func TestLoadSameLANExampleConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "examples", "config.same-lan.example.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Gateway.Mode != GatewayModeSameLAN {
		t.Fatalf("Gateway.Mode = %q", cfg.Gateway.Mode)
	}
	if cfg.DHCP.Enabled {
		t.Fatalf("DHCP.Enabled = true")
	}
	if cfg.Transparent.Mode != TransparentModeTUN {
		t.Fatalf("Transparent.Mode = %q", cfg.Transparent.Mode)
	}
}

func TestLoadSameWiFiDHCPGatewayMode(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
gateway:
  mode: "same_wifi_dhcp"
  interface: "en0"
  lan_ip: "192.168.1.20"
  upstream_interface: "en0"

dhcp:
  enabled: true
  range_start: "192.168.1.120"
  range_end: "192.168.1.199"

transparent:
  mode: "tun"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Gateway.Mode != GatewayModeSameWiFiDHCP {
		t.Fatalf("Gateway.Mode = %q", cfg.Gateway.Mode)
	}
	if !cfg.DHCP.Enabled {
		t.Fatal("DHCP.Enabled = false")
	}
}
