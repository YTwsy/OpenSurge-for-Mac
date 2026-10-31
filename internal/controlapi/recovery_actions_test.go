package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/mihomo"
)

func TestHelperKeepsListeningAfterPreparedRecoveryFailure(t *testing.T) {
	// Keep the Darwin Unix-socket path below its length limit.
	dir, err := os.MkdirTemp("", "omg-helper-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cfg := config.Default()
	cfg.Runtime.Dir = dir
	journal := filepath.Join(dir, "prepared-mihomo", "state.json")
	if err := os.MkdirAll(filepath.Dir(journal), 0o700); err != nil {
		t.Fatal(err)
	}
	broken := []byte("{interrupted-journal")
	if err := os.WriteFile(journal, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	socket := filepath.Join(dir, "helper.sock")
	go func() {
		done <- serveHelper(ctx, socket, dir, "", nil, nil, func(string) error {
			_, _, err := mihomo.LoadPrepared(cfg)
			return err
		})
	}()
	var conn net.Conn
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err = net.DialTimeout("unix", socket, 50*time.Millisecond)
		if err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("helper exited before accepting recovery requests: %v", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("helper socket unavailable after prepared recovery failure: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if err := json.NewEncoder(conn).Encode(HelperRequest{Action: "shell"}); err != nil {
		t.Fatal(err)
	}
	var response HelperResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Error != "action is not allowed" {
		t.Fatalf("helper stopped enforcing its action boundary: %+v", response)
	}
	if contents, err := os.ReadFile(journal); err != nil || string(contents) != string(broken) {
		t.Fatalf("failed recovery discarded its journal: %q, %v", contents, err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("helper did not stop after cancellation")
	}
}

func TestHelperRecoveryActionsDoNotRequireValidDeploymentOrPolicy(t *testing.T) {
	for _, policy := range []string{"missing", "invalid"} {
		t.Run(policy, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if policy == "invalid" {
				if err := os.WriteFile(filepath.Join(dir, "policy.json"), []byte("{broken"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			data := "gateway:\n  interface: en0\n  lan_ip: 192.0.2.1\ndns:\n  port: -1\ndevice_policy:\n  file: ./policy.json\n"
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, action := range []string{"stop", "config-apply-control", "network-set-dhcp", "dhcp-probe"} {
				cfg, err := loadHelperConfig(action, path)
				if err != nil || cfg.Gateway.Interface != "en0" {
					t.Fatalf("%s required unrelated deployment inputs: interface=%q, %v", action, cfg.Gateway.Interface, err)
				}
			}
			for _, action := range []string{"start", "reload", "network-set-manual"} {
				if _, err := loadHelperConfig(action, path); err == nil {
					t.Fatalf("%s accepted an invalid deployment", action)
				}
			}
		})
	}
}

func TestDHCPRecoveryStillRequiresConfiguredTarget(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "en0"
	if err := validateHelperDHCPProbe(cfg, "en0"); err != nil {
		t.Fatal(err)
	}
	for _, iface := range []string{"", " ", "en1"} {
		if err := validateHelperDHCPProbe(cfg, iface); err == nil {
			t.Fatalf("probe accepted interface %q", iface)
		}
		// Invalid targets fail before any network service query or write.
		if err := validateHelperNetworkTarget(t.Context(), cfg, "Wi-Fi", iface); err == nil {
			t.Fatalf("DHCP restore accepted interface %q", iface)
		}
	}
	cfg.Gateway.Interface = ""
	if err := validateHelperDHCPProbe(cfg, ""); err == nil {
		t.Fatal("probe accepted an empty configured interface")
	}
	if err := validateHelperNetworkTarget(t.Context(), cfg, "Wi-Fi", ""); err == nil {
		t.Fatal("DHCP restore accepted an empty configured interface")
	}
}

func TestDHCPProbeIgnoresUnappliedPolicyAndDeploymentErrors(t *testing.T) {
	server, _ := newTestServerWithNetwork(t)
	data, err := os.ReadFile(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("\ndns:\n  port: -1\ndevice_policy:\n  file: ./missing-policy.json\n")...)
	if err := os.WriteFile(server.configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SaveRecovery(RecoveryState{Stage: RecoveryMacStatic, Required: true}); err != nil {
		t.Fatal(err)
	}
	response := performAuthorized(server, http.MethodPost, "/api/v1/network/dhcp-probe", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("probe blocked by unrelated desired config: %d %s", response.Code, response.Body.String())
	}
	state, err := server.store.Recovery()
	if err != nil || state.Stage != RecoveryRouterDHCPDisabledConfirmed {
		t.Fatalf("probe did not complete its recovery step: %+v, %v", state, err)
	}
}

func TestHelperRestartTrustUsesOnlyCoreInputs(t *testing.T) {
	// These root-owned, read-only fixtures let the real ownership checks run
	// without elevating the test process or launching a core.
	binary, artifact := "/bin/sh", "/etc/hosts"
	if err := requireTrustedFile(binary, "/", true); err != nil {
		t.Skipf("root-owned executable fixture unavailable: %v", err)
	}
	if err := requireTrustedFile(artifact, "/", false); err != nil {
		t.Skipf("root-owned config fixture unavailable: %v", err)
	}
	cfg := config.Default()
	cfg.Mihomo.Binary, cfg.Mihomo.Config = binary, artifact
	cfg.Mihomo.Profile = artifact
	cfg.DHCP.Enabled = true
	cfg.DHCP.Binary = filepath.Join(t.TempDir(), "missing-dnsmasq")
	cfg.DevicePolicy.File = filepath.Join(t.TempDir(), "missing-policy.json")
	if err := requireTrustedRestartInputs(cfg, "/"); err != nil {
		t.Fatalf("restart required an unrelated input: %v", err)
	}
	if err := requireTrustedStartInputs(cfg, "/"); err == nil || !strings.Contains(err.Error(), "dhcp.binary") {
		t.Fatalf("full start lost its DHCP executable check: %v", err)
	}
	for _, input := range []string{"binary", "config", "profile"} {
		t.Run(input, func(t *testing.T) {
			candidate := cfg
			missing := filepath.Join(t.TempDir(), "missing")
			switch input {
			case "binary":
				candidate.Mihomo.Binary = missing
			case "config":
				candidate.Mihomo.Config = missing
			case "profile":
				candidate.Mihomo.Profile = missing
			}
			if err := requireTrustedRestartInputs(candidate, "/"); err == nil || !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("restart accepted missing %s: %v", input, err)
			}
		})
	}
	unsafe := filepath.Join(t.TempDir(), "untrusted-core.yaml")
	if err := os.WriteFile(unsafe, []byte("rules: []\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafe, 0o666); err != nil {
		t.Fatal(err)
	}
	cfg.Mihomo.Config = unsafe
	if err := requireTrustedRestartInputs(cfg, "/"); err == nil {
		t.Fatal("restart accepted a writable applied config")
	}
}
