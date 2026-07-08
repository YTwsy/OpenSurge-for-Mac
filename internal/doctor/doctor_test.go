package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestCheckGatewayInterfaceTopology(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "en0"
	cfg.Gateway.UpstreamInterface = " en0 "
	check := checkGatewayInterfaceTopology(cfg.Gateway)
	if check.OK {
		t.Fatalf("checkGatewayInterfaceTopology() OK = true")
	}
	if check.Message == "" {
		t.Fatalf("checkGatewayInterfaceTopology() missing failure message")
	}

	cfg.Gateway.Interface = "en7"
	cfg.Gateway.UpstreamInterface = "en0"
	check = checkGatewayInterfaceTopology(cfg.Gateway)
	if !check.OK {
		t.Fatalf("checkGatewayInterfaceTopology() OK = false: %s", check.Message)
	}

	cfg.Gateway.Mode = config.GatewayModeSameLAN
	cfg.Gateway.Interface = "en0"
	cfg.Gateway.UpstreamInterface = " en0 "
	check = checkGatewayInterfaceTopology(cfg.Gateway)
	if !check.OK {
		t.Fatalf("checkGatewayInterfaceTopology() same_lan OK = false: %s", check.Message)
	}

	cfg.Gateway.UpstreamInterface = "en7"
	check = checkGatewayInterfaceTopology(cfg.Gateway)
	if check.OK {
		t.Fatalf("checkGatewayInterfaceTopology() same_lan with different interfaces OK = true")
	}
}

func TestCheckInterfaceIPv4RejectsInvalidIP(t *testing.T) {
	check := checkInterfaceIPv4("en0", "not-an-ip")
	if check.OK {
		t.Fatalf("checkInterfaceIPv4() OK = true")
	}
	if check.Message != "invalid IPv4 address" {
		t.Fatalf("checkInterfaceIPv4() message = %q", check.Message)
	}
}

func TestCheckMihomoConfigRenderAcceptsImportedProfile(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	if err := os.WriteFile(profilePath, []byte("rules:\n  - MATCH,DIRECT\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = profilePath

	check := checkMihomoConfigRender(cfg)
	if !check.OK {
		t.Fatalf("checkMihomoConfigRender() OK = false: %s", check.Message)
	}
}

func TestCheckMihomoConfigRenderRejectsMissingImportedProfile(t *testing.T) {
	cfg := config.Default()
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = filepath.Join(t.TempDir(), "missing.yaml")

	check := checkMihomoConfigRender(cfg)
	if check.OK {
		t.Fatalf("checkMihomoConfigRender() OK = true")
	}
	if !strings.Contains(check.Message, "read imported mihomo profile") {
		t.Fatalf("checkMihomoConfigRender() message = %q", check.Message)
	}
}
