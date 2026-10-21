package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

func TestCustomDNSPreviewReadsAppliedBaseWithoutPersisting(t *testing.T) {
	for _, imported := range []bool{false, true} {
		name := "managed-overlay"
		if imported {
			name = "imported-without-library-snapshot"
		}
		t.Run(name, func(t *testing.T) {
			server := newTestServer(t)
			cfg := writeDNSPreviewAppliedBase(t, server.configPath, imported)
			before := map[string]string{}
			for _, path := range []string{server.configPath, cfg.Mihomo.Profile, policyWorkspaceBasePath(cfg)} {
				before[path] = fileDigest(path)
			}
			_, draftRevision, _ := server.loadCustomDNS()
			for _, enabled := range []bool{true, false, true} {
				doc := customDNSTestDocument()
				doc.Enabled = enabled
				doc.YAML = "hosts: {fresh.example: 192.0.2.19}"
				body, _ := json.Marshal(doc)
				response := performAuthorized(server, http.MethodPost, "/api/v1/custom-dns/preview", body)
				var preview CustomDNSPreview
				if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &preview) != nil || len(preview.Blockers) != 0 {
					t.Fatalf("preview failed: %s", response.Body.String())
				}
				if strings.Contains(preview.FinalYAML, "fresh.example") != enabled || strings.Contains(preview.FinalYAML, "nas.home.arpa") {
					t.Fatalf("preview stacked the applied DNS instead of using its base: %s", preview.FinalYAML)
				}
				if strings.Contains(response.Body.String(), "preview-private-secret") {
					t.Fatal("preview exposed non-DNS configuration")
				}
			}
			for path, digest := range before {
				if fileDigest(path) != digest {
					t.Fatalf("preview changed %s", path)
				}
			}
			_, afterRevision, _ := server.loadCustomDNS()
			if afterRevision != draftRevision {
				t.Fatal("preview saved the unsaved draft")
			}
			assertNoDNSPreviewFiles(t, cfg.Runtime.Dir)
			info, err := os.Stat(policyWorkspaceBasePath(cfg))
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("base permissions changed: %v %v", info, err)
			}
		})
	}
}

func TestCustomDNSPreviewRejectsStaleInvalidAndConcurrentSnapshots(t *testing.T) {
	server := newTestServer(t)
	cfg := writeDNSPreviewAppliedBase(t, server.configPath, false)
	doc := customDNSTestDocument()
	input := PolicyWorkspaceInput{Request: PolicyWorkspaceRequest{Action: "read"}, Revision: fileDigest(server.configPath), CustomDNS: &doc}
	stale := input
	stale.Revision = "stale"
	if _, err := previewCustomDNS(t.Context(), server.configPath, stale); err == nil || !strings.Contains(err.Error(), "revision conflict") {
		t.Fatalf("stale snapshot error: %v", err)
	}
	invalid := input
	invalid.Request.Action = "select"
	if _, err := previewCustomDNS(t.Context(), server.configPath, invalid); err == nil {
		t.Fatal("preview accepted a mutating workspace request")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := previewCustomDNS(ctx, server.configPath, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled preview error: %v", err)
	}
	lock, err := runtime.AcquireLifecycleLock(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, previewErr := previewCustomDNS(t.Context(), server.configPath, input)
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(previewErr, runtime.ErrLifecycleOperationInProgress) {
		t.Fatalf("preview raced a lifecycle operation: %v", previewErr)
	}
	// Composition failures are visible blockers, and still clean up private files.
	if err := os.WriteFile(policyWorkspaceBasePath(cfg), []byte("invalid JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := previewCustomDNS(t.Context(), server.configPath, input)
	if err != nil || len(preview.Blockers) != 1 || !strings.Contains(preview.Blockers[0], "read policy workspace base") {
		t.Fatalf("failed composition: %+v %v", preview, err)
	}
	assertNoDNSPreviewFiles(t, cfg.Runtime.Dir)
}

func writeDNSPreviewAppliedBase(t *testing.T, path string, imported bool) config.Config {
	t.Helper()
	cfg, err := config.LoadRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	cfg.LocalSystemDNS.Enabled = true
	cfg.Transparent.TUNAutoRoute = true
	cfg.Mihomo.Secret = "preview-private-secret"
	if imported {
		cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
		cfg.Mihomo.Profile = filepath.Join(filepath.Dir(path), "source.yaml")
		if err := writeAtomic(cfg.Mihomo.Profile, []byte("dns: {nameserver: [1.1.1.1]}\nproxies: [{name: private, type: socks5, server: 192.0.2.1, port: 1080, password: preview-private-secret}]\nrules: ['MATCH,DIRECT']\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	doc := customDNSTestDocument()
	candidate, base, err := workspaceCandidate(path, cfg, PolicyWorkspaceInput{CustomDNS: &doc, Overlay: []byte(policyWorkspaceOverlayFixture)})
	if err != nil {
		t.Fatal(err)
	}
	if err := persistPolicyWorkspaceCandidate(t.Context(), path, cfg, candidate, base); err != nil {
		t.Fatal(err)
	}
	if imported {
		if err := os.Remove(cfg.Mihomo.Profile); err != nil {
			t.Fatal(err)
		}
	}
	return candidate
}

func assertNoDNSPreviewFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(dir, ".custom-dns-preview-*"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("preview leaked private files: %v %v", entries, err)
	}
}

// Opt-in cross-UID gate: a real unprivileged Control Service request traverses
// the Unix socket to the root Helper. The fixture has no engines or networking.
func TestCustomDNSPreviewRootHelperPermissions(t *testing.T) {
	if dir := os.Getenv("OMG_DNS_PREVIEW_CHILD_DIR"); dir != "" {
		if os.Geteuid() == 0 {
			t.Fatal("permission probe must run unprivileged")
		}
		base := filepath.Join(dir, "runtime", "policy-workspace", "base.json")
		if _, err := os.ReadFile(base); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("fixture did not reproduce the private-base permission boundary: %v", err)
		}
		server, err := New(Options{ConfigPath: filepath.Join(dir, "config.yaml"), StoreDir: filepath.Join(dir, "store"), Runner: HelperClient{SocketPath: filepath.Join(dir, "helper.sock")}, Credentials: &memoryCredentialStore{}})
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(customDNSTestDocument())
		response := performAuthorized(server, http.MethodPost, "/api/v1/custom-dns/preview", body)
		var preview CustomDNSPreview
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &preview) != nil || len(preview.Blockers) != 0 || !strings.Contains(preview.FinalYAML, "nas.home.arpa") {
			t.Fatalf("unprivileged preview failed: %s", response.Body.String())
		}
		if strings.Contains(response.Body.String(), "preview-private-secret") {
			t.Fatal("Helper exposed private profile data")
		}
		return
	}
	if os.Getenv("OMG_TEST_ROOT_DNS_PREVIEW") != "1" || os.Geteuid() != 0 {
		t.Skip("set OMG_TEST_ROOT_DNS_PREVIEW=1 and run as root for the isolated cross-UID permission gate")
	}
	for _, imported := range []bool{false, true} {
		t.Run(map[bool]string{false: "managed", true: "imported"}[imported], func(t *testing.T) {
			tempRoot, err := filepath.EvalSymlinks("/tmp")
			if err != nil {
				t.Fatal(err)
			}
			dir, err := os.MkdirTemp(tempRoot, "opensurge-dns-permission-*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			if err := os.Chmod(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			cfg := config.Default()
			cfg.Runtime.Dir = filepath.Join(dir, "runtime")
			cfg.Transparent.Mode = "tun"
			path := filepath.Join(dir, "config.yaml")
			if err := writeAtomic(path, []byte(config.Render(cfg)), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg = writeDNSPreviewAppliedBase(t, path, imported)
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(cfg.Runtime.Dir, 0o755); err != nil {
				t.Fatal(err)
			}
			before := map[string]string{path: fileDigest(path), policyWorkspaceBasePath(cfg): fileDigest(policyWorkspaceBasePath(cfg)), cfg.Mihomo.Profile: fileDigest(cfg.Mihomo.Profile)}
			store := filepath.Join(dir, "store")
			if err := os.Mkdir(store, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chown(store, 65534, 65534); err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("unix", filepath.Join(dir, "helper.sock"))
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			if err := os.Chown(filepath.Join(dir, "helper.sock"), 0, 65534); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Join(dir, "helper.sock"), 0o660); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err == nil {
					handleHelperConn(t.Context(), conn, dir)
				}
			}()
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestCustomDNSPreviewRootHelperPermissions$", "-test.v")
			cmd.Env = append(os.Environ(), "OMG_DNS_PREVIEW_CHILD_DIR="+dir)
			cmd.Dir = dir
			cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
			output, err := cmd.CombinedOutput()
			listener.Close()
			<-done
			if err != nil {
				t.Fatalf("cross-UID preview: %v\n%s", err, output)
			}
			for file, digest := range before {
				if digest != fileDigest(file) {
					t.Fatalf("preview modified %s", file)
				}
			}
			assertNoDNSPreviewFiles(t, cfg.Runtime.Dir)
			info, err := os.Stat(policyWorkspaceBasePath(cfg))
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("private base permissions changed: %v %v", info, err)
			}
		})
	}
}
