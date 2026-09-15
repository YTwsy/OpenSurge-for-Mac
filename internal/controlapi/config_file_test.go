package controlapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestRedactConfigFileHidesPersistedCredentials(t *testing.T) {
	cfg := config.Default()
	cfg.Mihomo.Secret = "mihomo-secret-value"
	cfg.UpstreamProxy.Password = "upstream-password-value"
	content := redactConfigFile([]byte(config.Render(cfg)))

	if strings.Contains(content, cfg.Mihomo.Secret) || strings.Contains(content, cfg.UpstreamProxy.Password) {
		t.Fatalf("redacted config leaked a credential:\n%s", content)
	}
	if strings.Count(content, `"<redacted>"`) != 2 {
		t.Fatalf("redacted config has unexpected protected-value count: %d\n%s", strings.Count(content, `"<redacted>"`), content)
	}
}

func TestApplyConfigFilePreservesProtectedFieldsAndAllowsUpstreamGateway(t *testing.T) {
	configPath, current := writeConfigFileTestConfig(t)
	candidate := strings.Replace(config.Render(current), `upstream_gateway: "192.168.2.1"`, `upstream_gateway: "192.168.2.254"`, 1)
	candidate = strings.Replace(candidate, `secret: "mihomo-secret-value"`, `secret: "<redacted>"`, 1)
	candidate = strings.Replace(candidate, `password: "upstream-password-value"`, `password: "<redacted>"`, 1)

	revision, err := applyConfigFile(context.Background(), configPath, fileDigest(configPath), []byte(candidate))
	if err != nil {
		t.Fatalf("applyConfigFile() error = %v", err)
	}
	if revision == "" || revision == fileDigestBytes([]byte(config.Render(current))) {
		t.Fatalf("revision = %q, expected a changed config revision", revision)
	}

	updated, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load(saved config) error = %v", err)
	}
	if updated.Gateway.UpstreamGateway != "192.168.2.254" {
		t.Fatalf("upstream gateway = %q, want 192.168.2.254", updated.Gateway.UpstreamGateway)
	}
	if updated.Mihomo.Secret != current.Mihomo.Secret || updated.UpstreamProxy.Password != current.UpstreamProxy.Password {
		t.Fatalf("protected fields changed: mihomo secret=%q upstream password=%q", updated.Mihomo.Secret, updated.UpstreamProxy.Password)
	}
}

func TestApplyConfigFileRejectsProtectedFieldChangeWithoutWriting(t *testing.T) {
	configPath, current := writeConfigFileTestConfig(t)
	original, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	candidate := strings.Replace(config.Render(current), `secret: "mihomo-secret-value"`, `secret: "changed-secret"`, 1)

	_, err = applyConfigFile(context.Background(), configPath, fileDigest(configPath), []byte(candidate))
	if err == nil || !strings.Contains(err.Error(), "mihomo.secret is protected") {
		t.Fatalf("applyConfigFile() error = %v", err)
	}
	updated, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(updated, original) {
		t.Fatalf("protected-field rejection changed the authoritative config")
	}
}

func TestConfigFileEndpointReturnsRedactedContentAndConfirmsSave(t *testing.T) {
	server := newTestServer(t)
	cfg, err := config.Load(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Mihomo.Secret = "mihomo-secret-value"
	cfg.UpstreamProxy.Password = "upstream-password-value"
	if err := os.WriteFile(server.configPath, []byte(config.Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}

	get := performAuthorized(server, http.MethodGet, "/api/v1/config/file", nil)
	if get.Code != http.StatusOK {
		t.Fatalf("GET config file status=%d body=%s", get.Code, get.Body.String())
	}
	var file ConfigFile
	if err := json.Unmarshal(get.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	if file.Revision != fileDigest(server.configPath) || file.Path != filepath.ToSlash(server.configPath) {
		t.Fatalf("config file metadata = %#v", file)
	}
	if strings.Contains(file.Content, cfg.Mihomo.Secret) || strings.Contains(file.Content, cfg.UpstreamProxy.Password) || len(file.ProtectedFields) != 2 {
		t.Fatalf("GET config file leaked or omitted protected fields: %#v", file)
	}

	content := strings.Replace(file.Content, `upstream_gateway: ""`, `upstream_gateway: "192.168.2.1"`, 1)
	request := performAuthorizedRequest(server, http.MethodPut, "/api/v1/config/file", []byte(`{"content":`+mustJSON(t, content)+`}`), `"`+file.Revision+`"`)
	if request.Code != http.StatusOK {
		t.Fatalf("PUT config file status=%d body=%s", request.Code, request.Body.String())
	}
	var saved ConfigFile
	if err := json.Unmarshal(request.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Revision != fileDigest(server.configPath) || saved.Revision == file.Revision {
		t.Fatalf("saved config file metadata = %#v", saved)
	}
	updated, err := config.Load(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Gateway.UpstreamGateway != "192.168.2.1" || updated.Mihomo.Secret != cfg.Mihomo.Secret || updated.UpstreamProxy.Password != cfg.UpstreamProxy.Password {
		t.Fatalf("saved config = %#v", updated.Gateway)
	}
}

func writeConfigFileTestConfig(t *testing.T) (string, config.Config) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Gateway.Mode = config.GatewayModeSameLAN
	cfg.Gateway.UpstreamGateway = "192.168.2.1"
	cfg.DHCP.Enabled = false
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Mihomo.Secret = "mihomo-secret-value"
	cfg.UpstreamProxy.Password = "upstream-password-value"
	cfg.Runtime.Dir = filepath.Join(dir, "runtime")
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	path := filepath.Join(dir, "opensurge.yaml")
	if err := os.WriteFile(path, []byte(config.Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, cfg
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func performAuthorizedRequest(server *Server, method, path string, body []byte, ifMatch string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://127.0.0.1:61767"+path, bytes.NewReader(body))
	request.Host = "127.0.0.1:61767"
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("If-Match", ifMatch)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}
