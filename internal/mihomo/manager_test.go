package mihomo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestWaitForTUNWaitsForEnabledRuntimeState(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		enabled := requests.Add(1) >= 2
		_ = json.NewEncoder(w).Encode(map[string]any{"tun": map[string]any{"enable": enabled, "device": "utun123"}})
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Mihomo.APIAddr = server.URL
	cfg.Runtime.Dir = t.TempDir()
	manager := New(cfg, runtime.NewPaths(cfg))
	if err := os.MkdirAll(filepath.Dir(manager.paths.MihomoLog), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.paths.MihomoLog, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.waitForTUN(os.Getpid(), time.Second); err != nil {
		t.Fatal(err)
	}
	if requests.Load() < 2 {
		t.Fatalf("requests = %d", requests.Load())
	}
}

func TestWaitForTUNReturnsLoggedStartupFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tun":{"enable":false,"device":"utun123"}}`))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Mihomo.APIAddr = server.URL
	cfg.Runtime.Dir = t.TempDir()
	manager := New(cfg, runtime.NewPaths(cfg))
	if err := os.MkdirAll(filepath.Dir(manager.paths.MihomoLog), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.paths.MihomoLog, []byte("[error] Start TUN listening error: configure tun interface: add route: 1.0.0.0/8: file exists\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := manager.waitForTUN(os.Getpid(), time.Second)
	if err == nil || !strings.Contains(err.Error(), "1.0.0.0/8: file exists") {
		t.Fatalf("waitForTUN() error = %v", err)
	}
}

func TestEnrichTUNRouteErrorIgnoresUnrelatedErrors(t *testing.T) {
	const detail = "configure tun interface: permission denied"
	if got := enrichTUNRouteError(detail); got != detail {
		t.Fatalf("enrichTUNRouteError() = %q", got)
	}
}
