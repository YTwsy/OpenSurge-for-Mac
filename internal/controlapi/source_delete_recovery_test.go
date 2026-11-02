package controlapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestSourceDeleteIgnoresUnrelatedDeploymentErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{"invalid-dns-port", func(cfg *config.Config) { cfg.DNS.Port = -1 }},
		{"invalid-dhcp-range", func(cfg *config.Config) { cfg.DHCP.RangeStart = "198.51.100.120" }},
		{"missing-device-policy", func(cfg *config.Config) {
			cfg.DevicePolicy.File = filepath.Join(cfg.Runtime.Dir, "missing-policy.json")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t)
			source := importDeleteTestSource(t, server, "unused", "rules: [MATCH,DIRECT]\n")
			cfg, err := config.Load(server.configPath)
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&cfg)
			if err := os.WriteFile(server.configPath, []byte(config.Render(cfg)), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := config.Load(server.configPath); err == nil {
				t.Fatal("fixture must remain invalid for deployment")
			}
			revision := fileDigest(server.configPath)
			response := sourceDeleteRequest(server, source.ID, revision)
			if response.Code != http.StatusOK {
				t.Fatalf("unrelated deployment error blocked deletion: status=%d body=%s", response.Code, response.Body.String())
			}
			if fileDigest(server.configPath) != revision {
				t.Fatal("deletion rewrote the invalid desired configuration")
			}
			if sources, err := server.store.Sources(); err != nil || len(sources) != 0 {
				t.Fatalf("source was not deleted: sources=%v err=%v", sources, err)
			}
		})
	}
}

func TestSourceDeleteKeepsSelectedProtectionWithInvalidDeployment(t *testing.T) {
	for _, condition := range []string{"current", "historical", "composed"} {
		t.Run(condition, func(t *testing.T) {
			server := newTestServer(t)
			source := importDeleteTestSource(t, server, "selected", "rules: [MATCH,DIRECT]\n")
			unused := importDeleteTestSource(t, server, "unused", "rules: [MATCH,REJECT]\n")
			cfg, err := config.Load(server.configPath)
			if err != nil {
				t.Fatal(err)
			}
			cfg.DNS.Port = -1
			cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
			cfg.Mihomo.Profile = source.SnapshotPath
			if condition == "historical" {
				source = importDeleteTestSource(t, server, "selected", "rules: [MATCH,REJECT]\n")
			}
			if condition == "composed" {
				cfg.Mihomo.ProfileSourceDigest = source.Digest
				cfg.Mihomo.Profile = filepath.Join(t.TempDir(), "composed.yaml")
				if err := os.WriteFile(cfg.Mihomo.Profile, []byte("rules: [MATCH,DIRECT]\n# composed\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(server.configPath, []byte(config.Render(cfg)), 0o600); err != nil {
				t.Fatal(err)
			}
			revision := fileDigest(server.configPath)
			response := sourceDeleteRequest(server, source.ID, revision)
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "source_in_use") || !strings.Contains(response.Body.String(), "is selected") {
				t.Fatalf("selected source lost protection: status=%d body=%s", response.Code, response.Body.String())
			}
			if _, err := server.sourceByID(source.ID); err != nil {
				t.Fatal("selected source was removed")
			}
			if _, err := os.Stat(source.SnapshotPath); err != nil {
				t.Fatal("selected snapshot was removed")
			}
			response = sourceDeleteRequest(server, unused.ID, revision)
			if response.Code != http.StatusOK {
				t.Fatalf("unused source blocked: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSourceDeleteRejectsUncertainSourceIdentity(t *testing.T) {
	for _, condition := range []string{"unknown-mode", "missing-imported-path", "invalid-digest", "uppercase-digest", "managed-with-path", "managed-with-digest", "malformed-config"} {
		t.Run(condition, func(t *testing.T) {
			server := newTestServer(t)
			source := importDeleteTestSource(t, server, "unused", "rules: [MATCH,REJECT]\n")
			cfg, err := config.Load(server.configPath)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
			cfg.Mihomo.Profile = filepath.Join(t.TempDir(), "selected.yaml")
			if err := os.WriteFile(cfg.Mihomo.Profile, []byte("rules: [MATCH,DIRECT]\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			switch condition {
			case "unknown-mode":
				cfg.Mihomo.ProfileMode = "unknown"
			case "missing-imported-path":
				cfg.Mihomo.Profile = ""
			case "invalid-digest":
				cfg.Mihomo.ProfileSourceDigest = "invalid"
			case "uppercase-digest":
				cfg.Mihomo.ProfileSourceDigest = strings.Repeat("A", 64)
			case "managed-with-path":
				cfg.Mihomo.ProfileMode = config.MihomoProfileModeManaged
			case "managed-with-digest":
				cfg.Mihomo.ProfileMode = config.MihomoProfileModeManaged
				cfg.Mihomo.Profile = ""
				cfg.Mihomo.ProfileSourceDigest = source.Digest
			}
			data := []byte(config.Render(cfg))
			if condition == "malformed-config" {
				data = []byte("broken configuration\n")
			}
			if err := os.WriteFile(server.configPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			response := sourceDeleteRequest(server, source.ID, fileDigest(server.configPath))
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "cannot verify the selected source") {
				t.Fatalf("uncertain identity allowed deletion: status=%d body=%s", response.Code, response.Body.String())
			}
			if _, err := server.sourceByID(source.ID); err != nil {
				t.Fatal("rejected deletion removed the source record")
			}
			if _, err := os.Stat(source.SnapshotPath); err != nil {
				t.Fatal("rejected deletion removed the source snapshot")
			}
		})
	}
}
