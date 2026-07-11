package controlapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"open-mihomo-gateway/internal/macosnetwork"
)

func TestInspectSourceInventory(t *testing.T) {
	data := []byte(`proxies:
  - name: edge
    type: http
proxy-groups:
  - name: Main
    type: select
    proxies: [DIRECT, edge]
proxy-providers:
  subscription: {type: http, url: "https://example.com/sub"}
rule-providers:
  media: {type: http, behavior: domain, url: "https://example.com/rules"}
rules:
  - RULE-SET,media,Main
  - MATCH,DIRECT
`)
	inv, err := inspectSource(data, "mihomo_profile")
	if err != nil {
		t.Fatalf("inspectSource() error = %v", err)
	}
	if !inv.TerminalMatch || inv.RuleCount != 2 || len(inv.ProxyGroups) != 1 || inv.ProxyGroups[0] != "Main" {
		t.Fatalf("inventory = %#v", inv)
	}
}

func TestInspectSourceRejectsReservedNamespace(t *testing.T) {
	_, err := inspectSource([]byte(`proxy-groups:
  - name: device/phone/default
    type: select
    proxies: [DIRECT]
rules: ["MATCH,DIRECT"]
`), "mihomo_profile")
	if err == nil {
		t.Fatal("inspectSource() succeeded")
	}
}

func TestBootstrapIsOneTimeAndCreatesSession(t *testing.T) {
	server := newTestServer(t)
	bootstrap := server.BootstrapURL()
	request := httptest.NewRequest(http.MethodGet, bootstrap, nil)
	request.Host = "127.0.0.1:61767"
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusFound || len(response.Result().Cookies()) == 0 {
		t.Fatalf("first bootstrap status=%d cookies=%v", response.Code, response.Result().Cookies())
	}

	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("second bootstrap status=%d", response.Code)
	}
}

func TestBootstrapAllowsOnlyKnownWebPaths(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodGet, server.bootstrapURLFor("recovery"), nil)
	request.Host = "127.0.0.1:61767"
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Header().Get("Location") != "/network" {
		t.Fatalf("recovery bootstrap location=%q", response.Header().Get("Location"))
	}

	request = httptest.NewRequest(http.MethodGet, server.bootstrapURLFor("//evil.example"), nil)
	request.Host = "127.0.0.1:61767"
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Header().Get("Location") != "/dashboard" {
		t.Fatalf("unknown bootstrap location=%q", response.Header().Get("Location"))
	}
}

func TestRecoveryTransitionsPersist(t *testing.T) {
	server := newTestServer(t)
	response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/prepare", []byte(`{"network_service":"Wi-Fi"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("recovery status=%d body=%s", response.Code, response.Body.String())
	}
	state, err := server.store.Recovery()
	if err != nil || state.Stage != RecoveryPrepared || !state.Required {
		t.Fatalf("recovery=%#v err=%v", state, err)
	}
	if state.NetworkSnapshot == nil || state.NetworkSnapshot.Router != "192.168.1.1" {
		t.Fatalf("snapshot=%#v", state.NetworkSnapshot)
	}
	if _, err := os.Stat(filepath.Join(server.store.Dir(), "WIFI-DHCP-RECOVERY-CARD.txt")); err != nil {
		t.Fatalf("recovery card: %v", err)
	}
}

func TestGenericRecoveryPostCannotSkipSafetyChecks(t *testing.T) {
	server := newTestServer(t)
	body, _ := json.Marshal(RecoveryUpdate{Stage: RecoveryRouterDHCPDisabledConfirmed})
	response := performAuthorized(server, http.MethodPost, "/api/v1/recovery", body)
	if response.Code != http.StatusOK {
		t.Fatalf("recovery status=%d body=%s", response.Code, response.Body.String())
	}
	state, _ := server.store.Recovery()
	if state.Stage != RecoveryIdle {
		t.Fatalf("generic update advanced to %s", state.Stage)
	}
}

func TestSameWiFiNetworkRecoveryFlow(t *testing.T) {
	server, network := newTestServerWithNetwork(t)
	if response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/prepare", []byte(`{"network_service":"Wi-Fi"}`)); response.Code != http.StatusOK {
		t.Fatalf("prepare: %d %s", response.Code, response.Body.String())
	}
	if response := performAuthorized(server, http.MethodPost, "/api/v1/network/apply-static", nil); response.Code != http.StatusOK {
		t.Fatalf("static: %d %s", response.Code, response.Body.String())
	}
	if network.manual.IPv4 != "192.168.1.20" {
		t.Fatalf("manual=%#v", network.manual)
	}
	network.servers = []string{}
	if response := performAuthorized(server, http.MethodPost, "/api/v1/network/dhcp-probe", nil); response.Code != http.StatusOK {
		t.Fatalf("probe: %d %s", response.Code, response.Body.String())
	}
	state, _ := server.store.Recovery()
	if state.Stage != RecoveryRouterDHCPDisabledConfirmed {
		t.Fatalf("stage=%s", state.Stage)
	}
	state.Stage = RecoveryGatewayStopped
	_ = server.store.SaveRecovery(state)
	network.servers = []string{"192.168.1.1"}
	if response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/router-restored", nil); response.Code != http.StatusOK {
		t.Fatalf("router restored: %d %s", response.Code, response.Body.String())
	}
	if response := performAuthorized(server, http.MethodPost, "/api/v1/network/restore-dhcp", nil); response.Code != http.StatusOK {
		t.Fatalf("restore DHCP: %d %s", response.Code, response.Body.String())
	}
	state, _ = server.store.Recovery()
	if state.Stage != RecoveryComplete || state.Required || !network.dhcpRestored {
		t.Fatalf("final state=%#v network=%#v", state, network)
	}
}

func TestSafeDialRejectsLoopback(t *testing.T) {
	ctx := t.Context()
	_, err := safeDialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", "443"))
	if err == nil {
		t.Fatal("safeDialContext() accepted loopback")
	}
}

func TestStoreTokenPermissions(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Token(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(store.Dir(), "control-token"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode=%o", info.Mode().Perm())
	}
}

func TestHelperRejectsUserOwnedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("gateway: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires a non-root process")
	}
	if err := requireRootOwnedConfig(path); err == nil {
		t.Fatal("requireRootOwnedConfig() accepted a user-owned file")
	}
}

func TestHelperRejectsActionOutsideWhitelist(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	go handleHelperConn(t.Context(), serverConn, t.TempDir())
	if err := json.NewEncoder(clientConn).Encode(HelperRequest{Action: "shell"}); err != nil {
		t.Fatal(err)
	}
	var response HelperResponse
	if err := json.NewDecoder(clientConn).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Error != "action is not allowed" {
		t.Fatalf("response = %#v", response)
	}
}

func TestPublicSourcesKeepsEmptyArray(t *testing.T) {
	if sources := publicSources([]Source{}); sources == nil {
		t.Fatal("publicSources returned nil for an empty collection")
	}
}

func TestPublicSourcesRedactsFetchURLAndPath(t *testing.T) {
	public := publicSources([]Source{{Origin: "https://example.com/profile", FetchURL: "https://token@example.com/profile?secret=1", SnapshotPath: "/private/source.yaml"}})
	if public[0].FetchURL != "" || public[0].SnapshotPath != "" {
		t.Fatalf("public source leaked private fields: %#v", public[0])
	}
}

func newTestServer(t *testing.T) *Server {
	server, _ := newTestServerWithNetwork(t)
	return server
}

func newTestServerWithNetwork(t *testing.T) (*Server, *fakeNetworkRunner) {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`gateway:
  mode: "same_wifi_dhcp"
  interface: "en0"
  lan_ip: "192.168.1.20"
  upstream_interface: "en0"
dhcp:
  enabled: true
  range_start: "192.168.1.120"
  range_end: "192.168.1.199"
transparent:
  mode: "tun"
runtime:
  dir: "`+filepath.Join(dir, "runtime")+`"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	network := &fakeNetworkRunner{}
	discover := func(context.Context, string, string) (macosnetwork.Snapshot, error) {
		return macosnetwork.Snapshot{NetworkService: "Wi-Fi", Interface: "en0", IPv4: "192.168.1.20", SubnetMask: "255.255.255.0", Router: "192.168.1.1", DNS: []string{"192.168.1.1"}}, nil
	}
	server, err := New(Options{ConfigPath: configPath, Addr: "127.0.0.1:61767", StoreDir: filepath.Join(dir, "store"), Runner: fakeRunner{}, NetworkRunner: network, DiscoverNetwork: discover, PingRouter: func(context.Context, string) error { return nil }, Static: http.NotFoundHandler()})
	if err != nil {
		t.Fatal(err)
	}
	server.sessions["expired"] = time.Now().Add(-time.Minute)
	return server, network
}

type fakeRunner struct{}

func (fakeRunner) Run(_ context.Context, _, _ string) error { return nil }

type fakeNetworkRunner struct {
	manual       macosnetwork.ManualConfig
	dhcpRestored bool
	servers      []string
}

func (f *fakeNetworkRunner) SetManual(_ context.Context, _ string, cfg macosnetwork.ManualConfig) error {
	f.manual = cfg
	return nil
}
func (f *fakeNetworkRunner) SetDHCP(_ context.Context, _, _ string) error {
	f.dhcpRestored = true
	return nil
}
func (f *fakeNetworkRunner) ProbeDHCP(_ context.Context, _, _ string, _ time.Duration) ([]string, error) {
	return append([]string{}, f.servers...), nil
}

func performAuthorized(server *Server, method, path string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://127.0.0.1:61767"+path, bytes.NewReader(body))
	request.Host = "127.0.0.1:61767"
	request.Header.Set("Authorization", "Bearer "+server.token)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}
