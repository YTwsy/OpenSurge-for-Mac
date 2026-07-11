package installconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareRelocatesMutableAndExecutablePaths(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "profile.yaml")
	policy := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(profile, []byte("rules:\n  - MATCH,DIRECT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte(`{"devices":[],"profiles":[],"templates":[],"rule_sets":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.yaml")
	data := `gateway:
  mode: "same_wifi_dhcp"
  interface: "en0"
  lan_ip: "192.168.1.20"
  upstream_interface: "en0"
dhcp:
  enabled: true
  range_start: "192.168.1.120"
  range_end: "192.168.1.199"
device_policy:
  file: "` + policy + `"
mihomo:
  profile_mode: "imported"
  profile: "` + profile + `"
transparent:
  mode: "tun"
runtime:
  dir: "` + filepath.Join(dir, "runtime") + `"
`
	if err := os.WriteFile(source, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "installed")
	cfg, err := Prepare(source, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{cfg.DHCP.Binary, cfg.Mihomo.Binary, cfg.Mihomo.Config, cfg.Mihomo.Profile, cfg.DevicePolicy.File, cfg.Runtime.Dir} {
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == ".." || len(relative) > 3 && relative[:3] == "../" {
			t.Fatalf("path escaped install root: %s", path)
		}
	}
	if info, err := os.Stat(cfg.Mihomo.Profile); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("profile mode: info=%v err=%v", info, err)
	}
}

func TestValidatePackageSourceRejectsExternalInputs(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.yaml")
	if err := os.WriteFile(source, []byte(`mihomo:
  profile_mode: "imported"
  profile: "/tmp/profile.yaml"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePackageSource(source); err == nil {
		t.Fatal("imported package seed was accepted")
	}
}
