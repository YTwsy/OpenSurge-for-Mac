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
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/runtime"
)

func TestOverviewDoesNotGuessRuntimePathsAfterConfigReadFailure(t *testing.T) {
	server := newTestServer(t)
	if err := os.WriteFile(server.configPath, []byte("broken configuration\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) {
		t.Fatal("queried runtime with an unreadable configuration")
		return gateway.Status{}, nil
	}
	for _, path := range []string{"/api/v1/overview", "/api/v1/menubar"} {
		response := performAuthorized(server, http.MethodGet, path, nil)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("unreadable config became an ordinary status: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestControlConfigRepairsInvalidOldTopologyAndRejectsInvalidCandidate(t *testing.T) {
	server := newTestServer(t)
	cfg, err := config.Load(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DHCP.RangeStart = "198.51.100.120"
	old := []byte(config.Render(cfg))
	if err := os.WriteFile(server.configPath, old, 0o600); err != nil {
		t.Fatal(err)
	}
	get := performAuthorized(server, http.MethodGet, "/api/v1/config", nil)
	if get.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", get.Code, get.Body.String())
	}
	var current ControlConfig
	if err := json.Unmarshal(get.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if current.DHCP.RangeStart != cfg.DHCP.RangeStart {
		t.Fatal("repair form lost invalid saved value")
	}
	put := func(input ControlConfig) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:61767/api/v1/config", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+server.token)
		request.Header.Set("If-Match", `"`+current.Revision+`"`)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		return response
	}
	if response := put(current); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid candidate: %d %s", response.Code, response.Body.String())
	}
	unchanged, _ := os.ReadFile(server.configPath)
	if !bytes.Equal(old, unchanged) {
		t.Fatal("invalid candidate changed the saved configuration")
	}
	current.DHCP.RangeStart = "192.168.1.120"
	if response := put(current); response.Code != http.StatusOK {
		t.Fatalf("corrected candidate: %d %s", response.Code, response.Body.String())
	}
	if _, err := config.Load(server.configPath); err != nil {
		t.Fatalf("saved candidate invalid: %v", err)
	}
}

func TestControlRepairLockDoesNotRequireHealthyPreparedJournal(t *testing.T) {
	server := newTestServer(t)
	cfg, err := config.Load(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DHCP.RangeStart = "198.51.100.120"
	if err := os.WriteFile(server.configPath, []byte(config.Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}
	// The unresolved journal must survive a config-only save.
	journal := filepath.Join(cfg.Runtime.Dir, "prepared-mihomo", "state.json")
	if err := os.MkdirAll(filepath.Dir(journal), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journal, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	err = withConfigurationLock(server.configPath, false, func() error {
		called = true
		busy, err := gateway.LifecycleOperationInProgress(cfg)
		if err != nil {
			return err
		}
		if !busy {
			t.Fatal("repair escaped the shared lifecycle lock")
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("repair blocked: called=%v err=%v", called, err)
	}
	if got, _ := os.ReadFile(journal); string(got) != "broken" {
		t.Fatal("repair discarded unresolved prepared ownership")
	}
	if err := withConfigurationLifecycleLock(server.configPath, func() error { t.Fatal("resource-conflicting action ran"); return nil }); err == nil {
		t.Fatal("resource-conflicting change ignored the prepared journal")
	}
}

func TestStopAPIWorksWithInvalidDesiredTopology(t *testing.T) {
	server := newTestServer(t)
	cfg, err := config.Load(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DHCP.RangeStart = "198.51.100.120"
	if err := os.WriteFile(server.configPath, []byte(config.Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}
	response := performAuthorized(server, http.MethodPost, "/api/v1/gateway/stop", nil)
	if response.Code != http.StatusAccepted {
		t.Fatalf("stop blocked: %d %s", response.Code, response.Body.String())
	}
	var op Operation
	if err := json.Unmarshal(response.Body.Bytes(), &op); err != nil {
		t.Fatal(err)
	}
	waitForStoredOperation(t, server, op.ID, "succeeded")
}

func TestOverviewPreservesRuntimeStatusWithBadLeases(t *testing.T) {
	server := newTestServer(t)
	cfg, err := config.Load(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.LeaseFile, []byte("broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	response := performAuthorized(server, http.MethodGet, "/api/v1/overview", nil)
	var overview Overview
	if err := json.Unmarshal(response.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || overview.StatusError != "" || overview.Status.Gateway != "stopped" || overview.Status.DevicesError == "" {
		t.Fatalf("overview lost core status: %d %+v", response.Code, overview)
	}
	if !strings.Contains(strings.Join(overview.Warnings, "\n"), "device observations incomplete") {
		t.Fatalf("warnings=%v", overview.Warnings)
	}
	response = performAuthorized(server, http.MethodGet, "/api/v1/menubar", nil)
	var menu MenuBarStatus
	if err := json.Unmarshal(response.Body.Bytes(), &menu); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || menu.Gateway != "stopped" || menu.DevicesError == "" {
		t.Fatalf("menu=%+v", menu)
	}
}
