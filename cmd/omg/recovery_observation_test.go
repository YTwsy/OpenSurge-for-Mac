package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/mihomo"
)

func TestRecoveryObservationCommandsIgnoreInvalidFutureDeployment(t *testing.T) {
	previousManager, previousGroups := newGatewayManager, fetchProxyGroups
	previousConnections, previousProviders := fetchConnections, fetchProviders
	t.Cleanup(func() {
		newGatewayManager, fetchProxyGroups = previousManager, previousGroups
		fetchConnections, fetchProviders = previousConnections, previousProviders
	})
	// Snapshot inventory is deliberately unavailable. Its independent failures
	// must not block the persisted runtime status or require a live core.
	fetchProxyGroups = func(context.Context, config.Config) ([]mihomo.ProxyGroup, error) {
		return nil, errors.New("core unavailable")
	}
	fetchConnections = func(context.Context, config.Config) (mihomo.ConnectionsSnapshot, error) {
		return mihomo.ConnectionsSnapshot{}, errors.New("core unavailable")
	}
	fetchProviders = func(context.Context, config.Config) (mihomo.ProvidersSnapshot, error) {
		return mihomo.ProvidersSnapshot{}, errors.New("core unavailable")
	}
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Runtime.Dir = filepath.Join(dir, "runtime")
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	cfg.Mihomo.Binary = filepath.Join(dir, "missing-mihomo")
	cfg.DHCP.Binary = filepath.Join(dir, "missing-dnsmasq")
	cfg.Gateway.Interface, cfg.Gateway.UpstreamInterface = "missing-lan", "missing-wan"
	cfg.DHCP.RangeStart = "invalid"
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config.Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}
	managerRead := false
	newGatewayManager = func(loaded config.Config) gatewayManager {
		managerRead = true
		if loaded.Runtime.Dir != cfg.Runtime.Dir || loaded.DHCP.RangeStart != "invalid" {
			t.Fatalf("recovery changed persisted values: %+v", loaded)
		}
		return &fakeGatewayManager{status: gateway.Status{Gateway: "degraded", RuntimeState: "interrupted"}}
	}
	for _, command := range []string{"status", "logs", "snapshot", "doctor"} {
		t.Run(command, func(t *testing.T) {
			managerRead = false
			var exitCode int
			var stderr string
			stdout := captureStdout(t, func() {
				stderr = captureStderr(t, func() {
					exitCode = run([]string{command, "--config", configPath, "--format", "json"})
				})
			})
			if command == "doctor" {
				if exitCode != 1 || managerRead || !strings.Contains(stderr, "dhcp.range_start") {
					t.Fatalf("doctor lost deployment validation: exit=%d manager=%t stderr=%s", exitCode, managerRead, stderr)
				}
				return
			}
			if exitCode != 0 || !managerRead || stderr != "" {
				t.Fatalf("recovery command blocked: exit=%d manager=%t stderr=%s", exitCode, managerRead, stderr)
			}
			switch command {
			case "status":
				var result gateway.Status
				if err := json.Unmarshal([]byte(stdout), &result); err != nil || result.RuntimeState != "interrupted" {
					t.Fatalf("status=%s, error=%v", stdout, err)
				}
			case "logs":
				var result logsJSON
				if err := json.Unmarshal([]byte(stdout), &result); err != nil || result.LogsDir != filepath.Join(cfg.Runtime.Dir, "logs") {
					t.Fatalf("logs=%s, error=%v", stdout, err)
				}
			case "snapshot":
				var result snapshotJSON
				if err := json.Unmarshal([]byte(stdout), &result); err != nil || result.Status.RuntimeState != "interrupted" || result.Status.Error != "" || result.Doctor.Healthy {
					t.Fatalf("snapshot=%s, error=%v", stdout, err)
				}
			}
		})
	}
}
