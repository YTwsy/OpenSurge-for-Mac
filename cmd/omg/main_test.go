package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
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
	oldSelect := selectProxyGroup
	t.Cleanup(func() {
		selectProxyGroup = oldSelect
	})
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
