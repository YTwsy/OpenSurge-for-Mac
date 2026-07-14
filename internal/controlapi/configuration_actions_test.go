package controlapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestApplyProfileReloadsRunningGateway(t *testing.T) {
	configPath, original := writeProfileApplyTestConfig(t)
	reloaded := false
	deps := profileApplyDeps{
		geteuid:  func() int { return 0 },
		validate: func(config.Config) error { return nil },
		stateExists: func(config.Config) (bool, error) {
			return true, nil
		},
		reload: func(_ context.Context, candidate config.Config) error {
			reloaded = true
			if candidate.Mihomo.ProfileMode != config.MihomoProfileModeImported || candidate.Mihomo.Profile == "" {
				t.Fatalf("reload candidate profile = %#v", candidate.Mihomo)
			}
			return nil
		},
		start: func(context.Context, config.Config) error {
			t.Fatal("start called after successful reload")
			return nil
		},
	}
	result, err := applyProfile(t.Context(), configPath, fileDigest(configPath), profileApplyFixture(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded || !result.Reloaded || result.Revision == "" || result.Revision == fileDigestBytes(original) {
		t.Fatalf("result=%#v reloaded=%v", result, reloaded)
	}
	cfg, err := config.LoadRuntime(configPath)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := config.MihomoProfileDigest(cfg)
	if err != nil || digest != fileDigestBytes(profileApplyFixture()) {
		t.Fatalf("profile digest=%q err=%v", digest, err)
	}
}

func TestApplyProfileRestoresPreviousConfigAndGatewayAfterReloadFailure(t *testing.T) {
	configPath, original := writeProfileApplyTestConfig(t)
	stateChecks := 0
	restarted := false
	deps := profileApplyDeps{
		geteuid:  func() int { return 0 },
		validate: func(config.Config) error { return nil },
		stateExists: func(config.Config) (bool, error) {
			stateChecks++
			return stateChecks == 1, nil
		},
		reload: func(context.Context, config.Config) error {
			return errors.New("reload start failed after gateway stop: candidate failed")
		},
		start: func(_ context.Context, previous config.Config) error {
			restarted = true
			if previous.Mihomo.ProfileMode != config.MihomoProfileModeManaged || previous.Mihomo.Profile != "" {
				t.Fatalf("previous profile = %#v", previous.Mihomo)
			}
			return nil
		},
	}
	_, err := applyProfile(t.Context(), configPath, fileDigest(configPath), profileApplyFixture(), deps)
	if err == nil || !strings.Contains(err.Error(), "previous config restored") || !strings.Contains(err.Error(), "previous running gateway preserved or restored") {
		t.Fatalf("error = %v", err)
	}
	if !restarted {
		t.Fatal("previous gateway was not restarted")
	}
	current, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(current, original) {
		t.Fatalf("config was not restored\nwant:\n%s\ngot:\n%s", original, current)
	}
	profilePath := filepath.Join(filepath.Dir(configPath), "data", "imported-profile-"+fileDigestBytes(profileApplyFixture())[:16]+".yaml")
	if _, statErr := os.Stat(profilePath); !os.IsNotExist(statErr) {
		t.Fatalf("failed candidate profile remains: %v", statErr)
	}
}

func TestApplyProfileLeavesStoppedGatewayPendingForNextStart(t *testing.T) {
	configPath, _ := writeProfileApplyTestConfig(t)
	deps := profileApplyDeps{
		geteuid:     func() int { return 0 },
		validate:    func(config.Config) error { return nil },
		stateExists: func(config.Config) (bool, error) { return false, nil },
		reload: func(context.Context, config.Config) error {
			t.Fatal("reload called for stopped gateway")
			return nil
		},
		start: func(context.Context, config.Config) error {
			t.Fatal("start called while saving pending profile")
			return nil
		},
	}
	result, err := applyProfile(t.Context(), configPath, fileDigest(configPath), profileApplyFixture(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reloaded || result.Revision == "" {
		t.Fatalf("result = %#v", result)
	}
}

func writeProfileApplyTestConfig(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DHCP.Enabled = false
	cfg.Mihomo.Binary = filepath.Join(dir, "mihomo")
	cfg.Mihomo.Config = filepath.Join(dir, "runtime", "mihomo.yaml")
	cfg.Runtime.Dir = filepath.Join(dir, "runtime")
	path := filepath.Join(dir, "config.yaml")
	data := []byte(config.Render(cfg))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, data
}

func profileApplyFixture() []byte {
	return []byte("proxies:\n  - {name: edge, type: http, server: 127.0.0.1, port: 8080}\nproxy-groups:\n  - {name: Main, type: select, proxies: [edge, DIRECT]}\nrules:\n  - MATCH,Main\n")
}

func fileDigestBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
