package mihomo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

func TestConfigDirUsesGeneratedConfigDirForManagedMode(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Runtime.Dir = dir
	cfg.Mihomo.Config = filepath.Join(dir, "mihomo.yaml")

	manager := New(cfg, runtime.NewPaths(cfg))
	if got := manager.configDir(); got != dir {
		t.Fatalf("configDir() = %q, want %q", got, dir)
	}
}

func TestConfigDirUsesProfileDirForImportedMode(t *testing.T) {
	dir := t.TempDir()
	profileDir := filepath.Join(dir, "profile-home")
	cfg := config.Default()
	cfg.Runtime.Dir = filepath.Join(dir, "runtime")
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = filepath.Join(profileDir, "profile.yaml")

	manager := New(cfg, runtime.NewPaths(cfg))
	if got := manager.configDir(); got != profileDir {
		t.Fatalf("configDir() = %q, want %q", got, profileDir)
	}
}

func TestValidateConfigWithTimeoutReportsSlowEngine(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "mihomo")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf 'initializing geodata\\n'\nsleep 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := validateConfigWithTimeout(10*time.Millisecond, binary, dir, filepath.Join(dir, "mihomo.yaml"))
	if err == nil || !strings.Contains(err.Error(), "timed out after 10ms") {
		t.Fatalf("validateConfigWithTimeout() error = %v", err)
	}
}

func TestRuntimeEnvOnlyForAlwaysTUNIPv6(t *testing.T) {
	cfg := config.Default()
	manager := New(cfg, runtime.NewPaths(cfg))
	if got := manager.runtimeEnv(); got != nil {
		t.Fatalf("runtimeEnv() = %#v for disabled IPv6", got)
	}

	cfg.Transparent.TUNIPv6 = config.TUNIPv6Auto
	manager = New(cfg, runtime.NewPaths(cfg))
	if got := manager.runtimeEnv(); got != nil {
		t.Fatalf("runtimeEnv() = %#v for automatic IPv6", got)
	}

	cfg.Transparent.TUNIPv6 = config.TUNIPv6Always
	manager = New(cfg, runtime.NewPaths(cfg))
	if got := manager.runtimeEnv()["SKIP_SYSTEM_IPV6_CHECK"]; got != "1" {
		t.Fatalf("SKIP_SYSTEM_IPV6_CHECK = %q", got)
	}
}
