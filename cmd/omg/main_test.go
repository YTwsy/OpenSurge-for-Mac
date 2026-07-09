package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/mihomo"
)

func TestRenderMihomoCommandPrintsOverlayConfig(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	profile := `allow-lan: false
dns:
  enable: false
proxies:
  - name: Imported
    type: http
    server: 203.0.113.10
    port: 8080
proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - Imported
rules:
  - DOMAIN-SUFFIX,example.com,Proxy
  - MATCH,DIRECT
`
	if err := os.WriteFile(profilePath, []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
mihomo:
  profile_mode: "imported"
  profile: "` + profilePath + `"
  mixed_port: 17890
  api_addr: "127.0.0.1:19090"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"render-mihomo", "--config", configPath})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	for _, want := range []string{
		"mixed-port: 17890",
		"allow-lan: true",
		"external-controller: 127.0.0.1:19090",
		"proxies:",
		"- DOMAIN-SUFFIX,example.com,Proxy",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("render-mihomo output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "allow-lan: false") || strings.Contains(output, "enable: false") {
		t.Fatalf("render-mihomo output kept gateway-owned profile fields:\n%s", output)
	}
}

func TestStatusCommandPrintsJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
runtime:
  dir: "` + filepath.Join(dir, "runtime") + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"status", "--config", configPath, "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("status json invalid: %v\n%s", err, output)
	}
	if payload["gateway"] != "stopped" {
		t.Fatalf("gateway = %#v", payload["gateway"])
	}
	if payload["client_count"] != float64(0) {
		t.Fatalf("client_count = %#v", payload["client_count"])
	}
}

func TestStartCommandPrintsJSON(t *testing.T) {
	oldNewGatewayManager := newGatewayManager
	t.Cleanup(func() {
		newGatewayManager = oldNewGatewayManager
	})
	fake := &fakeGatewayManager{}
	newGatewayManager = func(cfg config.Config) gatewayManager {
		if cfg.Runtime.Dir == "" {
			t.Fatalf("runtime dir empty")
		}
		return fake
	}

	configPath := writeRuntimeConfig(t)
	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"start", "--config", configPath, "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}
	if !fake.startCalled {
		t.Fatalf("Start was not called")
	}

	var payload commandResultJSON
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("start json invalid: %v\n%s", err, output)
	}
	if payload.Command != "start" || !payload.OK || payload.ConfigPath != configPath {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestStartCommandPrintsJSONError(t *testing.T) {
	oldNewGatewayManager := newGatewayManager
	t.Cleanup(func() {
		newGatewayManager = oldNewGatewayManager
	})
	newGatewayManager = func(cfg config.Config) gatewayManager {
		return &fakeGatewayManager{startErr: errors.New("boom")}
	}

	configPath := writeRuntimeConfig(t)
	var exitCode int
	stderr := captureStderr(t, func() {
		exitCode = run([]string{"start", "--config", configPath, "--format", "json"})
	})
	if exitCode != 1 {
		t.Fatalf("run() exit = %d, stderr:\n%s", exitCode, stderr)
	}

	var payload commandErrorJSON
	if err := json.Unmarshal([]byte(stderr), &payload); err != nil {
		t.Fatalf("start error json invalid: %v\n%s", err, stderr)
	}
	if payload.Command != "start" || payload.OK || payload.Error != "start: boom" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestStopCommandPrintsJSON(t *testing.T) {
	oldNewGatewayManager := newGatewayManager
	t.Cleanup(func() {
		newGatewayManager = oldNewGatewayManager
	})
	fake := &fakeGatewayManager{}
	newGatewayManager = func(cfg config.Config) gatewayManager {
		return fake
	}

	configPath := writeRuntimeConfig(t)
	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"stop", "--config", configPath, "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}
	if !fake.stopCalled {
		t.Fatalf("Stop was not called")
	}

	var payload commandResultJSON
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("stop json invalid: %v\n%s", err, output)
	}
	if payload.Command != "stop" || !payload.OK || payload.ConfigPath != configPath {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestConfigLoadErrorPrintsJSON(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "missing.yaml")
	var exitCode int
	stderr := captureStderr(t, func() {
		exitCode = run([]string{"status", "--config", configPath, "--format", "json"})
	})
	if exitCode != 1 {
		t.Fatalf("run() exit = %d, stderr:\n%s", exitCode, stderr)
	}

	var payload commandErrorJSON
	if err := json.Unmarshal([]byte(stderr), &payload); err != nil {
		t.Fatalf("config error json invalid: %v\n%s", err, stderr)
	}
	if payload.Command != "status" || payload.OK || !strings.Contains(payload.Error, "config:") || !strings.Contains(payload.Error, "missing.yaml") {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestDoctorCommandPrintsJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
runtime:
  dir: "` + filepath.Join(dir, "runtime") + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"doctor", "--config", configPath, "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	var payload struct {
		Healthy bool `json:"healthy"`
		Checks  []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("doctor json invalid: %v\n%s", err, output)
	}
	if len(payload.Checks) == 0 {
		t.Fatalf("doctor checks empty")
	}
	foundRenderCheck := false
	for _, check := range payload.Checks {
		if check.Name == "mihomo config render" {
			foundRenderCheck = true
		}
	}
	if !foundRenderCheck {
		t.Fatalf("doctor json missing mihomo config render check: %#v", payload.Checks)
	}
}

func TestLeasesCommandPrintsJSON(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour).Unix()
	leaseBody := fmt.Sprintf("%d aa:bb:cc:dd:ee:ff 192.168.50.100 phone *\n", expires)
	if err := os.WriteFile(filepath.Join(runtimeDir, "dnsmasq.leases"), []byte(leaseBody), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
runtime:
  dir: "` + runtimeDir + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"leases", "--config", configPath, "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	var payload struct {
		Clients []struct {
			IP       string `json:"ip"`
			MAC      string `json:"mac"`
			Hostname string `json:"hostname"`
			Online   bool   `json:"online"`
		} `json:"clients"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("leases json invalid: %v\n%s", err, output)
	}
	if len(payload.Clients) != 1 {
		t.Fatalf("clients = %#v", payload.Clients)
	}
	client := payload.Clients[0]
	if client.IP != "192.168.50.100" || client.MAC != "aa:bb:cc:dd:ee:ff" || client.Hostname != "phone" || !client.Online {
		t.Fatalf("client = %#v", client)
	}
}

func TestLogsCommandPrintsJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	runtimeDir := filepath.Join(dir, "runtime")
	configBody := `
mihomo:
  config: "` + filepath.Join(runtimeDir, "mihomo.yaml") + `"
runtime:
  dir: "` + runtimeDir + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"logs", "--config", configPath, "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("logs json invalid: %v\n%s", err, output)
	}
	if payload["logs_dir"] != filepath.Join(runtimeDir, "logs") {
		t.Fatalf("logs_dir = %q", payload["logs_dir"])
	}
	if payload["mihomo_config"] != filepath.Join(runtimeDir, "mihomo.yaml") {
		t.Fatalf("mihomo_config = %q", payload["mihomo_config"])
	}
}

func TestLogsCommandPrintsTailJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	runtimeDir := filepath.Join(dir, "runtime")
	logDir := filepath.Join(runtimeDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "dnsmasq.log"), []byte("dns-1\ndns-2\ndns-3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configBody := `
mihomo:
  config: "` + filepath.Join(runtimeDir, "mihomo.yaml") + `"
runtime:
  dir: "` + runtimeDir + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"logs", "--config", configPath, "--tail", "2", "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	var payload struct {
		Recent []struct {
			Name   string   `json:"name"`
			Path   string   `json:"path"`
			Exists bool     `json:"exists"`
			Lines  []string `json:"lines"`
			Error  string   `json:"error"`
		} `json:"recent"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("logs json invalid: %v\n%s", err, output)
	}
	if len(payload.Recent) != 2 {
		t.Fatalf("recent = %#v", payload.Recent)
	}
	dnsmasq := payload.Recent[0]
	if dnsmasq.Name != "dnsmasq" || !dnsmasq.Exists || strings.Join(dnsmasq.Lines, ",") != "dns-2,dns-3" || dnsmasq.Error != "" {
		t.Fatalf("dnsmasq tail = %#v", dnsmasq)
	}
	mihomo := payload.Recent[1]
	if mihomo.Name != "mihomo" || mihomo.Exists || len(mihomo.Lines) != 0 || mihomo.Error != "" {
		t.Fatalf("mihomo tail = %#v", mihomo)
	}
}

func TestLogsCommandRejectsNegativeTail(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
runtime:
  dir: "` + filepath.Join(dir, "runtime") + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	stderr := captureStderr(t, func() {
		exitCode = run([]string{"logs", "--config", configPath, "--tail", "-1"})
	})
	if exitCode != 2 {
		t.Fatalf("run() exit = %d, stderr:\n%s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "tail must be greater than or equal to 0") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestLogsCommandRejectsNegativeTailJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
runtime:
  dir: "` + filepath.Join(dir, "runtime") + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	stderr := captureStderr(t, func() {
		exitCode = run([]string{"logs", "--config", configPath, "--tail", "-1", "--format", "json"})
	})
	if exitCode != 2 {
		t.Fatalf("run() exit = %d, stderr:\n%s", exitCode, stderr)
	}
	var payload commandErrorJSON
	if err := json.Unmarshal([]byte(stderr), &payload); err != nil {
		t.Fatalf("logs error json invalid: %v\n%s", err, stderr)
	}
	if payload.Command != "logs" || payload.OK || payload.Error != "logs: tail must be greater than or equal to 0" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestSnapshotCommandPrintsPartialJSON(t *testing.T) {
	oldFetchGroups := fetchProxyGroups
	oldFetchConnections := fetchConnections
	oldFetchProviders := fetchProviders
	t.Cleanup(func() {
		fetchProxyGroups = oldFetchGroups
		fetchConnections = oldFetchConnections
		fetchProviders = oldFetchProviders
	})
	fetchProxyGroups = func(ctx context.Context, cfg config.Config) ([]mihomo.ProxyGroup, error) {
		if cfg.Mihomo.APIAddr != "127.0.0.1:9090" {
			t.Fatalf("api_addr = %q", cfg.Mihomo.APIAddr)
		}
		return []mihomo.ProxyGroup{
			{Name: "Proxy", Type: "Selector", Selected: "DIRECT", Options: []string{"DIRECT", "HK"}},
		}, nil
	}
	fetchConnections = func(ctx context.Context, cfg config.Config) (mihomo.ConnectionsSnapshot, error) {
		return mihomo.ConnectionsSnapshot{}, errors.New("mihomo API unavailable")
	}
	fetchProviders = func(ctx context.Context, cfg config.Config) (mihomo.ProvidersSnapshot, error) {
		return mihomo.ProvidersSnapshot{
			ProxyProviders: []mihomo.ProxyProvider{
				{Name: "demo", Type: "Proxy", VehicleType: "File", ProxyCount: 1, Proxies: []mihomo.ProviderProxy{{Name: "HK", Type: "Http", Alive: true}}},
			},
		}, nil
	}

	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, "runtime")
	logDir := filepath.Join(runtimeDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour).Unix()
	leaseBody := fmt.Sprintf("%d aa:bb:cc:dd:ee:ff 192.168.50.100 phone *\n", expires)
	if err := os.WriteFile(filepath.Join(runtimeDir, "dnsmasq.leases"), []byte(leaseBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "mihomo.log"), []byte("mihomo-1\nmihomo-2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
mihomo:
  api_addr: "127.0.0.1:9090"
  config: "` + filepath.Join(runtimeDir, "mihomo.yaml") + `"
runtime:
  dir: "` + runtimeDir + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"snapshot", "--config", configPath, "--tail", "1", "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	var payload struct {
		Status struct {
			Gateway     string `json:"gateway"`
			ClientCount int    `json:"client_count"`
		} `json:"status"`
		Doctor struct {
			Healthy bool `json:"healthy"`
			Checks  []struct {
				Name string `json:"name"`
			} `json:"checks"`
		} `json:"doctor"`
		Leases struct {
			Clients []struct {
				IP string `json:"ip"`
			} `json:"clients"`
			Error string `json:"error"`
		} `json:"leases"`
		Logs struct {
			Recent []struct {
				Name  string   `json:"name"`
				Lines []string `json:"lines"`
			} `json:"recent"`
		} `json:"logs"`
		Mihomo struct {
			APIAddr  string `json:"api_addr"`
			Policies struct {
				Available bool `json:"available"`
				Groups    []struct {
					Name string `json:"name"`
				} `json:"groups"`
				Error string `json:"error"`
			} `json:"policies"`
			Connections struct {
				Available   bool   `json:"available"`
				Connections []any  `json:"connections"`
				Error       string `json:"error"`
			} `json:"connections"`
			Providers struct {
				Available      bool `json:"available"`
				ProxyProviders []struct {
					Name       string `json:"name"`
					ProxyCount int    `json:"proxy_count"`
					Proxies    []struct {
						Name  string `json:"name"`
						Alive bool   `json:"alive"`
					} `json:"proxies"`
				} `json:"proxy_providers"`
				Error string `json:"error"`
			} `json:"providers"`
		} `json:"mihomo"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("snapshot json invalid: %v\n%s", err, output)
	}
	if payload.Status.Gateway != "stopped" || payload.Status.ClientCount != 1 {
		t.Fatalf("status = %#v", payload.Status)
	}
	if len(payload.Doctor.Checks) == 0 {
		t.Fatalf("doctor checks empty")
	}
	if len(payload.Leases.Clients) != 1 || payload.Leases.Clients[0].IP != "192.168.50.100" || payload.Leases.Error != "" {
		t.Fatalf("leases = %#v", payload.Leases)
	}
	if len(payload.Logs.Recent) != 2 || payload.Logs.Recent[1].Name != "mihomo" || strings.Join(payload.Logs.Recent[1].Lines, ",") != "mihomo-2" {
		t.Fatalf("logs = %#v", payload.Logs)
	}
	if payload.Mihomo.APIAddr != "127.0.0.1:9090" {
		t.Fatalf("api_addr = %q", payload.Mihomo.APIAddr)
	}
	if !payload.Mihomo.Policies.Available || len(payload.Mihomo.Policies.Groups) != 1 || payload.Mihomo.Policies.Groups[0].Name != "Proxy" || payload.Mihomo.Policies.Error != "" {
		t.Fatalf("policies = %#v", payload.Mihomo.Policies)
	}
	if payload.Mihomo.Connections.Available || !strings.Contains(payload.Mihomo.Connections.Error, "mihomo API unavailable") || len(payload.Mihomo.Connections.Connections) != 0 {
		t.Fatalf("connections = %#v", payload.Mihomo.Connections)
	}
	if !payload.Mihomo.Providers.Available || len(payload.Mihomo.Providers.ProxyProviders) != 1 || payload.Mihomo.Providers.ProxyProviders[0].Name != "demo" || payload.Mihomo.Providers.ProxyProviders[0].Proxies[0].Name != "HK" || !payload.Mihomo.Providers.ProxyProviders[0].Proxies[0].Alive {
		t.Fatalf("providers = %#v", payload.Mihomo.Providers)
	}
}

func TestSnapshotCommandReportsLeaseParseErrorsInJSON(t *testing.T) {
	oldFetchGroups := fetchProxyGroups
	oldFetchConnections := fetchConnections
	oldFetchProviders := fetchProviders
	t.Cleanup(func() {
		fetchProxyGroups = oldFetchGroups
		fetchConnections = oldFetchConnections
		fetchProviders = oldFetchProviders
	})
	fetchProxyGroups = func(ctx context.Context, cfg config.Config) ([]mihomo.ProxyGroup, error) {
		return nil, errors.New("mihomo API unavailable")
	}
	fetchConnections = func(ctx context.Context, cfg config.Config) (mihomo.ConnectionsSnapshot, error) {
		return mihomo.ConnectionsSnapshot{}, errors.New("mihomo API unavailable")
	}
	fetchProviders = func(ctx context.Context, cfg config.Config) (mihomo.ProvidersSnapshot, error) {
		return mihomo.ProvidersSnapshot{}, errors.New("mihomo API unavailable")
	}

	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "dnsmasq.leases"), []byte("not-a-valid-lease\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
runtime:
  dir: "` + runtimeDir + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"snapshot", "--config", configPath, "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	var payload struct {
		Status struct {
			Error string `json:"error"`
		} `json:"status"`
		Leases struct {
			Clients []any  `json:"clients"`
			Error   string `json:"error"`
		} `json:"leases"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("snapshot json invalid: %v\n%s", err, output)
	}
	if !strings.Contains(payload.Status.Error, "expected dnsmasq lease fields") {
		t.Fatalf("status error = %q", payload.Status.Error)
	}
	if len(payload.Leases.Clients) != 0 || !strings.Contains(payload.Leases.Error, "expected dnsmasq lease fields") {
		t.Fatalf("leases = %#v", payload.Leases)
	}
}

func TestSnapshotCommandRejectsNegativeTail(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
runtime:
  dir: "` + filepath.Join(dir, "runtime") + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	stderr := captureStderr(t, func() {
		exitCode = run([]string{"snapshot", "--config", configPath, "--tail", "-1"})
	})
	if exitCode != 2 {
		t.Fatalf("run() exit = %d, stderr:\n%s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "tail must be greater than or equal to 0") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestPoliciesCommandPrintsJSON(t *testing.T) {
	oldFetch := fetchProxyGroups
	t.Cleanup(func() {
		fetchProxyGroups = oldFetch
	})
	fetchProxyGroups = func(ctx context.Context, cfg config.Config) ([]mihomo.ProxyGroup, error) {
		if cfg.Mihomo.APIAddr != "127.0.0.1:9090" {
			t.Fatalf("api_addr = %q", cfg.Mihomo.APIAddr)
		}
		if cfg.Mihomo.Secret != "test-secret" {
			t.Fatalf("secret = %q", cfg.Mihomo.Secret)
		}
		return []mihomo.ProxyGroup{
			{Name: "Proxy", Type: "Selector", Selected: "DIRECT", Options: []string{"DIRECT", "HK"}},
		}, nil
	}

	configPath := writeAPIConfig(t, "127.0.0.1:9090")
	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"policies", "--config", configPath, "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	var payload struct {
		Groups []struct {
			Name     string   `json:"name"`
			Selected string   `json:"selected"`
			Options  []string `json:"options"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("policies json invalid: %v\n%s", err, output)
	}
	if len(payload.Groups) != 1 || payload.Groups[0].Name != "Proxy" || payload.Groups[0].Selected != "DIRECT" {
		t.Fatalf("groups = %#v", payload.Groups)
	}
	if strings.Join(payload.Groups[0].Options, ",") != "DIRECT,HK" {
		t.Fatalf("options = %#v", payload.Groups[0].Options)
	}
}

func TestPolicySelectCommandCallsMihomoAPI(t *testing.T) {
	oldFetch := fetchProxyGroups
	oldSelect := selectProxyGroup
	t.Cleanup(func() {
		fetchProxyGroups = oldFetch
		selectProxyGroup = oldSelect
	})
	fetchProxyGroups = func(ctx context.Context, cfg config.Config) ([]mihomo.ProxyGroup, error) {
		if cfg.Mihomo.APIAddr != "127.0.0.1:9090" {
			t.Fatalf("api_addr = %q", cfg.Mihomo.APIAddr)
		}
		if cfg.Mihomo.Secret != "test-secret" {
			t.Fatalf("secret = %q", cfg.Mihomo.Secret)
		}
		return []mihomo.ProxyGroup{
			{Name: "Proxy Group", Type: "Selector", Selected: "DIRECT", Options: []string{"DIRECT", "HK"}},
		}, nil
	}
	var selectedGroup string
	var selectedPolicy string
	selectProxyGroup = func(ctx context.Context, cfg config.Config, groupName, selected string) error {
		if cfg.Mihomo.APIAddr != "127.0.0.1:9090" {
			t.Fatalf("api_addr = %q", cfg.Mihomo.APIAddr)
		}
		selectedGroup = groupName
		selectedPolicy = selected
		return nil
	}

	configPath := writeAPIConfig(t, "127.0.0.1:9090")
	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"policy-select", "--config", configPath, "--group", "Proxy Group", "--policy", "HK", "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("policy-select json invalid: %v\n%s", err, output)
	}
	if payload["group"] != "Proxy Group" || payload["selected"] != "HK" {
		t.Fatalf("payload = %#v", payload)
	}
	if selectedGroup != "Proxy Group" || selectedPolicy != "HK" {
		t.Fatalf("selected = %q/%q", selectedGroup, selectedPolicy)
	}
}

func TestPolicySelectCommandRejectsUnknownPolicy(t *testing.T) {
	oldFetch := fetchProxyGroups
	oldSelect := selectProxyGroup
	t.Cleanup(func() {
		fetchProxyGroups = oldFetch
		selectProxyGroup = oldSelect
	})
	fetchProxyGroups = func(ctx context.Context, cfg config.Config) ([]mihomo.ProxyGroup, error) {
		return []mihomo.ProxyGroup{
			{Name: "Proxy", Type: "Selector", Selected: "DIRECT", Options: []string{"DIRECT", "HK"}},
		}, nil
	}
	selectCalled := false
	selectProxyGroup = func(ctx context.Context, cfg config.Config, groupName, selected string) error {
		selectCalled = true
		return nil
	}

	configPath := writeAPIConfig(t, "127.0.0.1:9090")
	var exitCode int
	stderr := captureStderr(t, func() {
		exitCode = run([]string{"policy-select", "--config", configPath, "--group", "Proxy", "--policy", "Missing"})
	})
	if exitCode == 0 {
		t.Fatalf("run() exit = 0, want non-zero")
	}
	if selectCalled {
		t.Fatalf("selectProxyGroup called for unknown policy")
	}
	for _, want := range []string{`policy "Missing" is not a member of group "Proxy"`, "DIRECT, HK"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr)
		}
	}
}

func TestValidatePolicySelection(t *testing.T) {
	groups := []mihomo.ProxyGroup{
		{Name: "Fallback", Type: "Fallback", Selected: "JP", Options: []string{"JP", "US"}},
		{Name: "Proxy", Type: "Selector", Selected: "DIRECT", Options: []string{"DIRECT", "HK"}},
	}

	for _, tc := range []struct {
		name      string
		groupName string
		selected  string
		wantError string
	}{
		{name: "accepts member", groupName: "Proxy", selected: "HK"},
		{name: "requires group", groupName: " ", selected: "HK", wantError: "policy group is required"},
		{name: "requires policy", groupName: "Proxy", selected: "", wantError: "selected policy is required"},
		{name: "unknown group", groupName: "Missing", selected: "HK", wantError: `policy group "Missing" not found`},
		{name: "unknown policy", groupName: "Proxy", selected: "US", wantError: `policy "US" is not a member of group "Proxy"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePolicySelection(groups, tc.groupName, tc.selected)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("validatePolicySelection() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validatePolicySelection() error = nil, want %q", tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("validatePolicySelection() error = %q, want %q", err.Error(), tc.wantError)
			}
		})
	}
}

func TestConnectionsCommandPrintsJSON(t *testing.T) {
	oldFetch := fetchConnections
	t.Cleanup(func() {
		fetchConnections = oldFetch
	})
	fetchConnections = func(ctx context.Context, cfg config.Config) (mihomo.ConnectionsSnapshot, error) {
		if cfg.Mihomo.APIAddr != "127.0.0.1:9090" {
			t.Fatalf("api_addr = %q", cfg.Mihomo.APIAddr)
		}
		return mihomo.ConnectionsSnapshot{
			UploadTotal:   100,
			DownloadTotal: 200,
			Connections: []mihomo.Connection{
				{
					ID:          "abc",
					Upload:      10,
					Download:    20,
					Chains:      []string{"Proxy", "demo-proxy"},
					Rule:        "Domain",
					RulePayload: "example.com",
					Metadata:    map[string]any{"host": "example.com", "destinationPort": "443"},
				},
			},
		}, nil
	}

	configPath := writeAPIConfig(t, "127.0.0.1:9090")
	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"connections", "--config", configPath, "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	var payload struct {
		UploadTotal   int `json:"upload_total"`
		DownloadTotal int `json:"download_total"`
		Connections   []struct {
			ID          string         `json:"id"`
			RulePayload string         `json:"rule_payload"`
			Metadata    map[string]any `json:"metadata"`
		} `json:"connections"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("connections json invalid: %v\n%s", err, output)
	}
	if payload.UploadTotal != 100 || payload.DownloadTotal != 200 {
		t.Fatalf("totals = %#v", payload)
	}
	if len(payload.Connections) != 1 || payload.Connections[0].ID != "abc" || payload.Connections[0].RulePayload != "example.com" {
		t.Fatalf("connections = %#v", payload.Connections)
	}
	if payload.Connections[0].Metadata["host"] != "example.com" {
		t.Fatalf("metadata = %#v", payload.Connections[0].Metadata)
	}
}

func TestProvidersCommandPrintsJSON(t *testing.T) {
	oldFetch := fetchProviders
	t.Cleanup(func() {
		fetchProviders = oldFetch
	})
	fetchProviders = func(ctx context.Context, cfg config.Config) (mihomo.ProvidersSnapshot, error) {
		if cfg.Mihomo.APIAddr != "127.0.0.1:9090" {
			t.Fatalf("api_addr = %q", cfg.Mihomo.APIAddr)
		}
		return mihomo.ProvidersSnapshot{
			ProxyProviders: []mihomo.ProxyProvider{
				{Name: "demo", Type: "Proxy", VehicleType: "File", ProxyCount: 1, Proxies: []mihomo.ProviderProxy{{Name: "HK", Type: "Http", Alive: true}}},
			},
			RuleProviders: []mihomo.RuleProvider{
				{Name: "cn", Type: "Rule", VehicleType: "File", Behavior: "classical", RuleCount: 2},
			},
		}, nil
	}

	configPath := writeAPIConfig(t, "127.0.0.1:9090")
	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"providers", "--config", configPath, "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}

	var payload struct {
		ProxyProviders []struct {
			Name       string `json:"name"`
			ProxyCount int    `json:"proxy_count"`
			Proxies    []struct {
				Name  string `json:"name"`
				Alive bool   `json:"alive"`
			} `json:"proxies"`
		} `json:"proxy_providers"`
		RuleProviders []struct {
			Name      string `json:"name"`
			RuleCount int    `json:"rule_count"`
		} `json:"rule_providers"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("providers json invalid: %v\n%s", err, output)
	}
	if len(payload.ProxyProviders) != 1 || payload.ProxyProviders[0].Name != "demo" || payload.ProxyProviders[0].ProxyCount != 1 || payload.ProxyProviders[0].Proxies[0].Name != "HK" || !payload.ProxyProviders[0].Proxies[0].Alive {
		t.Fatalf("proxy providers = %#v", payload.ProxyProviders)
	}
	if len(payload.RuleProviders) != 1 || payload.RuleProviders[0].Name != "cn" || payload.RuleProviders[0].RuleCount != 2 {
		t.Fatalf("rule providers = %#v", payload.RuleProviders)
	}
}

func TestFormatConnections(t *testing.T) {
	output := formatConnections(mihomo.ConnectionsSnapshot{
		UploadTotal:   100,
		DownloadTotal: 200,
		Connections: []mihomo.Connection{
			{
				ID:          "abc",
				Chains:      []string{"Proxy", "demo-proxy"},
				Rule:        "Domain",
				RulePayload: "example.com",
				Metadata:    map[string]any{"host": "example.com", "destinationPort": "443"},
			},
		},
	})
	for _, want := range []string{
		"Connections: 1",
		"Upload total: 100 bytes",
		"Download total: 200 bytes",
		"abc example.com:443 Proxy -> demo-proxy Domain(example.com)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("formatConnections output missing %q:\n%s", want, output)
		}
	}
}

func TestValidateMihomoCommandUsesImportedProfileDir(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "mihomo-args.txt")
	t.Setenv("MIHOMO_ARGS_FILE", argsPath)
	mihomoBinary := filepath.Join(dir, "fake-mihomo")
	script := `#!/bin/sh
: > "$MIHOMO_ARGS_FILE"
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$MIHOMO_ARGS_FILE"
done
exit 0
`
	if err := os.WriteFile(mihomoBinary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	profileDir := filepath.Join(dir, "profile home")
	providerDir := filepath.Join(profileDir, "providers")
	if err := os.MkdirAll(providerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providerDir, "cn.yaml"), []byte("payload:\n  - DOMAIN,example.org\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(profileDir, "profile.yaml")
	profile := `rule-providers:
  cn:
    type: file
    behavior: classical
    path: ./providers/cn.yaml
rules:
  - RULE-SET,cn,DIRECT
  - MATCH,DIRECT
`
	if err := os.WriteFile(profilePath, []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}

	runtimeDir := filepath.Join(dir, "runtime")
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
mihomo:
  binary: "` + mihomoBinary + `"
  config: "` + filepath.Join(runtimeDir, "mihomo.yaml") + `"
  profile_mode: "imported"
  profile: "` + profilePath + `"
runtime:
  dir: "` + runtimeDir + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	output := captureStdout(t, func() {
		exitCode = run([]string{"validate-mihomo", "--config", configPath})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "mihomo config valid: "+filepath.Join(runtimeDir, "mihomo.yaml")) {
		t.Fatalf("validate-mihomo output = %q", output)
	}

	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(argsData)), "\n")
	wantArgs := []string{"-d", profileDir, "-t", "-f", filepath.Join(runtimeDir, "mihomo.yaml")}
	if strings.Join(args, "\n") != strings.Join(wantArgs, "\n") {
		t.Fatalf("mihomo args = %#v, want %#v", args, wantArgs)
	}

	rendered, err := os.ReadFile(filepath.Join(runtimeDir, "mihomo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	wantProviderPath := "path: " + filepath.Join(profileDir, "providers", "cn.yaml")
	if !strings.Contains(string(rendered), wantProviderPath) {
		t.Fatalf("rendered config missing %q:\n%s", wantProviderPath, rendered)
	}

	output = captureStdout(t, func() {
		exitCode = run([]string{"validate-mihomo", "--config", configPath, "--format", "json"})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, output:\n%s", exitCode, output)
	}
	var payload struct {
		Valid        bool   `json:"valid"`
		MihomoConfig string `json:"mihomo_config"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("validate-mihomo json invalid: %v\n%s", err, output)
	}
	if !payload.Valid || payload.MihomoConfig != filepath.Join(runtimeDir, "mihomo.yaml") {
		t.Fatalf("payload = %#v", payload)
	}
}

func writeAPIConfig(t *testing.T, apiAddr string) string {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
mihomo:
  api_addr: "` + apiAddr + `"
  secret: "test-secret"
runtime:
  dir: "` + filepath.Join(dir, "runtime") + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func writeRuntimeConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configBody := `
runtime:
  dir: "` + filepath.Join(dir, "runtime") + `"
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return configPath
}

type fakeGatewayManager struct {
	startCalled bool
	stopCalled  bool
	status      gateway.Status
	startErr    error
	stopErr     error
	statusErr   error
}

func (m *fakeGatewayManager) Start(ctx context.Context) error {
	m.startCalled = true
	return m.startErr
}

func (m *fakeGatewayManager) Stop(ctx context.Context) error {
	m.stopCalled = true
	return m.stopErr
}

func (m *fakeGatewayManager) Status(ctx context.Context) (gateway.Status, error) {
	return m.status, m.statusErr
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = old
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	defer func() {
		os.Stderr = old
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
