package controlapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/macosnetwork"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
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

func TestSourceRequestUsesMihomoCompatibleUserAgent(t *testing.T) {
	request, err := newSourceRequest(t.Context(), "https://example.com/subscription")
	if err != nil {
		t.Fatal(err)
	}
	if got := request.Header.Get("User-Agent"); got != "clash.meta" {
		t.Fatalf("User-Agent = %q, want clash.meta", got)
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
	cardPath := filepath.Join(server.store.Dir(), "WIFI-DHCP-RECOVERY-CARD.txt")
	if _, err := os.Stat(cardPath); err != nil {
		t.Fatalf("recovery card: %v", err)
	}
	card, err := os.ReadFile(cardPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"同一 LAN DHCP 恢复卡", "原始 IPv4：192.168.1.20", "原始路由器：192.168.1.1", "原始 DNS：192.168.1.1", "正常路径必须先确认路由器 DHCP 已恢复并通过 OFFER 探测", "跳过 OFFER 探测并恢复 Mac 自动 DHCP"} {
		if !strings.Contains(string(card), want) {
			t.Fatalf("recovery card missing %q:\n%s", want, card)
		}
	}

	response = performAuthorized(server, http.MethodGet, "/api/v1/recovery/card", nil)
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Disposition"), "inline;") || !strings.Contains(response.Body.String(), "恢复顺序") {
		t.Fatalf("inline recovery card: status=%d disposition=%q body=%s", response.Code, response.Header().Get("Content-Disposition"), response.Body.String())
	}
	response = performAuthorized(server, http.MethodGet, "/api/v1/recovery/card?download=1", nil)
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("download recovery card: status=%d disposition=%q", response.Code, response.Header().Get("Content-Disposition"))
	}
}

func TestPreparedRecoveryCanBeDiscardedBeforeNetworkChanges(t *testing.T) {
	server := newTestServer(t)
	if response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/prepare", []byte(`{"network_service":"Wi-Fi"}`)); response.Code != http.StatusOK {
		t.Fatalf("prepare: %d %s", response.Code, response.Body.String())
	}
	response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/discard", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("discard: %d %s", response.Code, response.Body.String())
	}
	state, err := server.store.Recovery()
	if err != nil || state.Stage != RecoveryIdle || state.Required || state.NetworkSnapshot != nil {
		t.Fatalf("recovery=%#v err=%v", state, err)
	}
	if _, err := os.Stat(filepath.Join(server.store.Dir(), "WIFI-DHCP-RECOVERY-CARD.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery card still exists: %v", err)
	}
	missing := performAuthorized(server, http.MethodGet, "/api/v1/recovery/card", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing card status=%d body=%s", missing.Code, missing.Body.String())
	}
}

func TestRecoveryPrepareRollsBackWhenOfflineCardCannotBeWritten(t *testing.T) {
	server := newTestServer(t)
	cardPath := filepath.Join(server.store.Dir(), "WIFI-DHCP-RECOVERY-CARD.txt")
	if err := os.Mkdir(cardPath, 0o700); err != nil {
		t.Fatal(err)
	}
	response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/prepare", []byte(`{"network_service":"Wi-Fi"}`))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "recovery_card_failed") {
		t.Fatalf("prepare status=%d body=%s", response.Code, response.Body.String())
	}
	state, err := server.store.Recovery()
	if err != nil || state.Stage != RecoveryIdle || state.Required || state.NetworkSnapshot != nil {
		t.Fatalf("recovery=%#v err=%v", state, err)
	}
}

func TestPreparedRecoveryDiscardIsRejectedAfterNetworkChangesBegin(t *testing.T) {
	server, _ := newTestServerWithNetwork(t)
	if response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/prepare", []byte(`{"network_service":"Wi-Fi"}`)); response.Code != http.StatusOK {
		t.Fatalf("prepare: %d %s", response.Code, response.Body.String())
	}
	if response := performAuthorized(server, http.MethodPost, "/api/v1/network/apply-static", nil); response.Code != http.StatusOK {
		t.Fatalf("apply static: %d %s", response.Code, response.Body.String())
	}
	response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/discard", nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("discard status=%d body=%s", response.Code, response.Body.String())
	}
	state, _ := server.store.Recovery()
	if state.Stage != RecoveryMacStatic || !state.Required {
		t.Fatalf("recovery=%#v", state)
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

func TestManualRecoveryFinishRequiresExplicitConfirmation(t *testing.T) {
	server, network := newTestServerWithNetwork(t)
	state := RecoveryState{
		Stage: RecoveryGatewayStopped, Required: true,
		NetworkSnapshot: &macosnetwork.Snapshot{NetworkService: "Wi-Fi", Interface: "en0"},
	}
	if err := server.store.SaveRecovery(state); err != nil {
		t.Fatal(err)
	}

	response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/manual-finish", []byte(`{"router_dhcp_restored_confirmed":false}`))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("manual finish status=%d body=%s", response.Code, response.Body.String())
	}
	state, _ = server.store.Recovery()
	if state.Stage != RecoveryGatewayStopped || !state.Required || network.dhcpRestored {
		t.Fatalf("manual fallback advanced without confirmation: state=%#v network=%#v", state, network)
	}
}

func TestManualRecoveryFinishRestoresMacDHCPAndRecordsOverride(t *testing.T) {
	server, network := newTestServerWithNetwork(t)
	state := RecoveryState{
		Stage: RecoveryGatewayStopped, Required: true, RecoveryNotes: "client evidence saved",
		NetworkSnapshot: &macosnetwork.Snapshot{NetworkService: "Wi-Fi", Interface: "en0"},
	}
	if err := server.store.SaveRecovery(state); err != nil {
		t.Fatal(err)
	}

	response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/manual-finish", []byte(`{"router_dhcp_restored_confirmed":true}`))
	if response.Code != http.StatusOK {
		t.Fatalf("manual finish status=%d body=%s", response.Code, response.Body.String())
	}
	state, _ = server.store.Recovery()
	if state.Stage != RecoveryComplete || state.Required || !network.dhcpRestored {
		t.Fatalf("manual finish state=%#v network=%#v", state, network)
	}
	if !strings.Contains(state.RecoveryNotes, "OFFER evidence skipped") || !strings.Contains(state.RecoveryNotes, "client evidence saved") {
		t.Fatalf("manual finish notes=%q", state.RecoveryNotes)
	}
}

func TestRecoveryPrepareRejectsGatewayIPv4OutsideRouterSubnet(t *testing.T) {
	server, _ := newTestServerWithNetwork(t)
	cfg, err := config.Load(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Gateway.LANIP = "192.168.50.1"
	cfg.DNS.Listen = cfg.Gateway.LANIP
	cfg.DHCP.RangeStart, cfg.DHCP.RangeEnd = "192.168.50.100", "192.168.50.199"
	if err := os.WriteFile(server.configPath, []byte(config.Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}

	response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/prepare", []byte(`{"network_service":"Wi-Fi"}`))
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "configured Mac LAN IPv4 192.168.50.1") {
		t.Fatalf("prepare status=%d body=%s", response.Code, response.Body.String())
	}
	state, _ := server.store.Recovery()
	if state.Stage != RecoveryIdle || state.Required {
		t.Fatalf("recovery state=%#v", state)
	}
	if _, err := os.Stat(filepath.Join(server.store.Dir(), "WIFI-DHCP-RECOVERY-CARD.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prepared recovery card was not cleared: %v", err)
	}
}

func TestRecoveryPrepareRequiresSavedSameWiFiTopology(t *testing.T) {
	server, _ := newTestServerWithNetwork(t)
	cfg, err := config.Load(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Gateway.Mode = config.GatewayModeIsolatedLAN
	if err := os.WriteFile(server.configPath, []byte(config.Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}

	response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/prepare", []byte(`{"network_service":"Wi-Fi"}`))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "same_wifi_config_required") {
		t.Fatalf("prepare status=%d body=%s", response.Code, response.Body.String())
	}
	state, _ := server.store.Recovery()
	if state.Stage != RecoveryIdle || state.Required {
		t.Fatalf("recovery state=%#v", state)
	}
}

func TestControlConfigCanCorrectPreparedRecoveryBeforeNetworkChanges(t *testing.T) {
	server, _ := newTestServerWithNetwork(t)
	if response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/prepare", []byte(`{"network_service":"Wi-Fi"}`)); response.Code != http.StatusOK {
		t.Fatalf("prepare: %d %s", response.Code, response.Body.String())
	}
	get := performAuthorized(server, http.MethodGet, "/api/v1/config", nil)
	if get.Code != http.StatusOK {
		t.Fatalf("config: %d %s", get.Code, get.Body.String())
	}
	var current ControlConfig
	if err := json.Unmarshal(get.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	current.Gateway.LANIP, current.DNS.Listen = "192.168.1.21", "192.168.1.21"
	payload, _ := json.Marshal(current)
	request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:61767/api/v1/config", bytes.NewReader(payload))
	request.Host = "127.0.0.1:61767"
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("If-Match", `"`+current.Revision+`"`)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("config update: %d %s", response.Code, response.Body.String())
	}
	state, _ := server.store.Recovery()
	if state.Stage != RecoveryIdle || state.Required {
		t.Fatalf("recovery state=%#v", state)
	}
	if _, err := os.Stat(filepath.Join(server.store.Dir(), "WIFI-DHCP-RECOVERY-CARD.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prepared recovery card was not cleared after config save: %v", err)
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

func TestOperationHistoryIsNewestFirstAndLimited(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	older := Operation{ID: "older", Kind: "start", State: "succeeded", UpdatedAt: time.Now().Add(-time.Minute)}
	newer := Operation{ID: "newer", Kind: "stop", State: "failed", UpdatedAt: time.Now()}
	if err := store.SaveOperation(older); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOperation(newer); err != nil {
		t.Fatal(err)
	}
	operations, err := store.Operations(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 || operations[0].ID != "newer" {
		t.Fatalf("operations=%#v", operations)
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

func TestHelperAllowlistIncludesOnlyNamedReloadAction(t *testing.T) {
	if !helperActionAllowed("reload") {
		t.Fatal("reload is not available to the privileged helper")
	}
	for _, action := range []string{"hot-reload", "restart", "shell"} {
		if helperActionAllowed(action) {
			t.Fatalf("unexpected helper action %q", action)
		}
	}
}

func TestTrustedPathRejectsEscapesAndUserOwnedFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "mihomo")
	if err := os.WriteFile(outside, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := trustedPathWithinRoot(outside, root); err == nil {
		t.Fatal("outside path was accepted")
	}
	inside := filepath.Join(root, "mihomo")
	if err := os.WriteFile(inside, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 {
		if err := requireTrustedFile(inside, root, true); err == nil {
			t.Fatal("user-owned executable was accepted")
		}
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

func TestHTTPSSourceMetadataNeverPersistsFetchURL(t *testing.T) {
	server := newTestServer(t)
	source, err := server.importReader("subscription", "mihomo_profile", "https://example.com/profile", strings.NewReader("rules:\n  - MATCH,DIRECT\n"))
	if err != nil {
		t.Fatal(err)
	}
	if source.FetchURL != "" {
		t.Fatal("import result retained a fetch URL")
	}
	stored, err := server.store.Sources()
	if err != nil || len(stored) != 1 || stored[0].FetchURL != "" {
		t.Fatalf("stored sources = %#v err=%v", stored, err)
	}
}

func TestLegacySourceCredentialMigratesOutOfJSON(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSources([]Source{{ID: "source-1", FetchURL: "https://token@example.com/profile?secret=1"}}); err != nil {
		t.Fatal(err)
	}
	credentials := &memoryCredentialStore{}
	if err := migrateSourceCredentials(t.Context(), store, credentials); err != nil {
		t.Fatal(err)
	}
	if value, err := credentials.Get(t.Context(), "source-1"); err != nil || value != "https://token@example.com/profile?secret=1" {
		t.Fatalf("credential=%q err=%v", value, err)
	}
	sources, err := store.Sources()
	if err != nil || sources[0].FetchURL != "" {
		t.Fatalf("sources=%#v err=%v", sources, err)
	}
	raw, err := os.ReadFile(filepath.Join(store.Dir(), "sources.json"))
	if err != nil || strings.Contains(string(raw), "secret=1") {
		t.Fatalf("legacy secret remains: %s err=%v", raw, err)
	}
}

func TestSourceRefreshPreservesAppliedVersionAndBuildsInventoryDiff(t *testing.T) {
	server := newTestServer(t)
	first, err := server.importReader("home", "mihomo_profile", "file:home.yaml", strings.NewReader("proxies:\n  - {name: old, type: direct}\nproxy-groups:\n  - {name: Main, type: select, proxies: [DIRECT]}\nrules:\n  - MATCH,DIRECT\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadRuntime(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = first.SnapshotPath
	if err := os.WriteFile(server.configPath, []byte(config.Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := runtime.NewPaths(cfg)
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{ProfileDigest: first.Digest, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	second, err := server.importReader("home", "mihomo_profile", "file:home.yaml", strings.NewReader("proxies:\n  - {name: new, type: direct}\nproxy-groups:\n  - {name: Main, type: select, proxies: [DIRECT]}\nrules:\n  - DOMAIN,example.com,DIRECT\n  - MATCH,DIRECT\n"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Applied || len(second.Versions) != 1 || !second.Versions[0].Applied {
		t.Fatalf("versions = %#v", second)
	}
	if second.Diff.PreviousDigest != first.Digest || len(second.Diff.ProxiesAdded) != 1 || second.Diff.ProxiesAdded[0] != "new" || second.Diff.RuleCountDelta != 1 {
		t.Fatalf("diff = %#v", second.Diff)
	}
	public := publicSources([]Source{second})[0]
	if public.Versions[0].SnapshotPath != "" {
		t.Fatal("public version leaked snapshot path")
	}
}

func TestSourceApplyDelegatesAuthoritativeEngineValidationToRunner(t *testing.T) {
	server := newTestServer(t)
	source, err := server.importReader("home", "mihomo_profile", "file:home.yaml", strings.NewReader("rules:\n  - MATCH,DIRECT\n"))
	if err != nil {
		t.Fatal(err)
	}
	revision := fileDigest(server.configPath)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:61767/api/v1/sources/"+source.ID+"/apply", nil)
	request.Host = "127.0.0.1:61767"
	request.SetPathValue("id", source.ID)
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("If-Match", `"`+revision+`"`)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("apply status=%d body=%s", response.Code, response.Body.String())
	}

	server.configRunner = fakeConfigurationRunner{profileErr: errors.New("mihomo config validation failed: geodata unavailable")}
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "mihomo_validation_failed") {
		t.Fatalf("engine failure status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDevicePolicyUsesOptimisticRevisionAndConfigurationRunner(t *testing.T) {
	server := newTestServer(t)
	get := performAuthorized(server, http.MethodGet, "/api/v1/device-policy", nil)
	if get.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", get.Code, get.Body.String())
	}
	var document DevicePolicyResponse
	if err := json.Unmarshal(get.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	conflict := performAuthorized(server, http.MethodPut, "/api/v1/device-policy", []byte(`{"devices":[],"profiles":[],"templates":[],"rule_sets":[]}`))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:61767/api/v1/device-policy", strings.NewReader(`{"devices":[],"profiles":[],"templates":[],"rule_sets":[]}`))
	request.Host = "127.0.0.1:61767"
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("If-Match", `"`+document.Revision+`"`)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestChangedDeviceIDsTracksResolvedPrivateProfileChanges(t *testing.T) {
	applied := device.PolicySet{
		Devices: []device.ManagedDevice{
			{ID: "alice", MAC: "aa:bb:cc:dd:ee:01", IPv4: "192.168.1.121", Profile: "alice-policy"},
			{ID: "bob", MAC: "aa:bb:cc:dd:ee:02", IPv4: "192.168.1.122", Profile: "bob-policy"},
		},
		Profiles: []device.Profile{
			{ID: "alice-policy", DefaultPolicies: []string{"DIRECT"}},
			{ID: "bob-policy", DefaultPolicies: []string{"DIRECT"}},
		},
	}
	desired := applied
	desired.Profiles = append([]device.Profile(nil), applied.Profiles...)
	desired.Profiles[0].Rules = []device.Rule{{ID: "youtube", Match: device.RuleMatch{Domains: []string{"youtube.example"}}, Action: "REJECT"}}
	changed := changedDeviceIDs(desired, applied)
	if !reflect.DeepEqual(changed, []string{"alice"}) {
		t.Fatalf("changed devices=%v", changed)
	}
}

func TestControlConfigUsesRevisionAndAppliesTopology(t *testing.T) {
	server := newTestServer(t)
	get := performAuthorized(server, http.MethodGet, "/api/v1/config", nil)
	if get.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", get.Code, get.Body.String())
	}
	var current ControlConfig
	if err := json.Unmarshal(get.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	current.Gateway.Mode = config.GatewayModeSameLAN
	current.DHCP.Enabled = false
	requestBody, _ := json.Marshal(current)
	conflict := performAuthorized(server, http.MethodPut, "/api/v1/config", requestBody)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:61767/api/v1/config", bytes.NewReader(requestBody))
	request.Host = "127.0.0.1:61767"
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("If-Match", `"`+current.Revision+`"`)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", response.Code, response.Body.String())
	}
	updated, err := config.Load(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Gateway.Mode != config.GatewayModeSameLAN || updated.DHCP.Enabled {
		t.Fatalf("updated config=%#v", updated)
	}
}

func TestGatewayReloadPreservesActiveTakeoverStage(t *testing.T) {
	server := newTestServer(t)
	cfg, err := config.LoadRuntime(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{PIDDNSMasq: os.Getpid(), PIDMihomo: os.Getpid(), StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SaveRecovery(RecoveryState{Stage: RecoveryClientValidated, Required: true}); err != nil {
		t.Fatal(err)
	}
	runner := &recordingActionRunner{}
	server.runner = runner
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:61767/api/v1/gateway/reload", nil)
	request.Host = "127.0.0.1:61767"
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("Idempotency-Key", "reload-success")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("reload status=%d body=%s", response.Code, response.Body.String())
	}
	waitForStoredOperation(t, server, "reload-success", "succeeded")
	if runner.action != "reload" {
		t.Fatalf("runner action=%q", runner.action)
	}
	if err := runtime.RemoveState(paths.StateFile); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || runner.count != 1 {
		t.Fatalf("idempotent reload status=%d runner count=%d body=%s", response.Code, runner.count, response.Body.String())
	}
	recovery, _ := server.store.Recovery()
	if recovery.Stage != RecoveryClientValidated {
		t.Fatalf("recovery stage=%q", recovery.Stage)
	}
}

func TestGatewayReloadStopFailurePreservesActiveTakeoverStage(t *testing.T) {
	server := newTestServer(t)
	cfg, err := config.LoadRuntime(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{PIDDNSMasq: os.Getpid(), PIDMihomo: os.Getpid(), StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SaveRecovery(RecoveryState{Stage: RecoveryClientValidated, Required: true}); err != nil {
		t.Fatal(err)
	}
	server.runner = actionRunnerFunc(func(_ context.Context, _, _ string) error {
		_ = runtime.RemoveState(paths.StateFile)
		return errors.New("reload stop failed: pf unload failed")
	})
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:61767/api/v1/gateway/reload", nil)
	request.Host = "127.0.0.1:61767"
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("Idempotency-Key", "reload-stop-failed")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("reload status=%d body=%s", response.Code, response.Body.String())
	}
	waitForStoredOperation(t, server, "reload-stop-failed", "failed")
	recovery, _ := server.store.Recovery()
	if recovery.Stage != RecoveryClientValidated {
		t.Fatalf("recovery stage=%q", recovery.Stage)
	}
}

func TestGatewayReloadFailureAfterStopReturnsToRestartableTakeoverStage(t *testing.T) {
	server := newTestServer(t)
	cfg, err := config.LoadRuntime(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{PIDDNSMasq: os.Getpid(), PIDMihomo: os.Getpid(), StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SaveRecovery(RecoveryState{Stage: RecoveryGatewayActive, Required: true}); err != nil {
		t.Fatal(err)
	}
	server.runner = actionRunnerFunc(func(_ context.Context, action, _ string) error {
		if action != "reload" {
			t.Fatalf("action=%q", action)
		}
		if err := runtime.RemoveState(paths.StateFile); err != nil {
			t.Fatal(err)
		}
		return errors.New("restart failed")
	})
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:61767/api/v1/gateway/reload", nil)
	request.Host = "127.0.0.1:61767"
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("Idempotency-Key", "reload-failed")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("reload status=%d body=%s", response.Code, response.Body.String())
	}
	waitForStoredOperation(t, server, "reload-failed", "failed")
	recovery, _ := server.store.Recovery()
	if recovery.Stage != RecoveryRouterDHCPDisabledConfirmed || !strings.Contains(recovery.RecoveryNotes, "reload failed") {
		t.Fatalf("recovery=%#v", recovery)
	}
}

func TestGatewayReloadRejectsStoppedGateway(t *testing.T) {
	server := newTestServer(t)
	unauthorized := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:61767/api/v1/gateway/reload", nil)
	unauthorized.Host = "127.0.0.1:61767"
	unauthorizedResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized reload status=%d", unauthorizedResponse.Code)
	}
	response := performAuthorized(server, http.MethodPost, "/api/v1/gateway/reload", nil)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "gateway_not_running") {
		t.Fatalf("reload status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestControlConfigShowsMihomoDNSForLegacyEmptyUpstream(t *testing.T) {
	cfg := config.Default()
	cfg.DNS.Upstream = ""
	if got := controlConfigFrom(cfg, "revision").DNS.Upstream; got != config.MihomoDNSUpstream {
		t.Fatalf("DNS upstream = %q, want %q", got, config.MihomoDNSUpstream)
	}
}

func TestStateEventCarriesConfigGatewayAndRecoveryState(t *testing.T) {
	server := newTestServer(t)
	state, err := server.stateEvent(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision == "" || state.Gateway == "" || state.Recovery.Stage != RecoveryIdle {
		t.Fatalf("state event = %#v", state)
	}
}

func TestClientAcceptanceRequiresLeaseDNSAndTUNEvidence(t *testing.T) {
	server := newTestServer(t)
	cfg, err := config.LoadRuntime(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	paths := runtime.NewPaths(cfg)
	if err := os.MkdirAll(paths.LogDir, 0o700); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour).Unix()
	if err := os.WriteFile(paths.LeaseFile, []byte(fmt.Sprintf("%d aa:bb:cc:dd:ee:ff 192.168.1.121 phone *\n", expires)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.DNSMasqLog, []byte("DHCPACK(en0) 192.168.1.121 aa:bb:cc:dd:ee:ff phone\nquery[A] example.com from 192.168.1.121\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.MihomoLog, []byte("[TCP] 192.168.1.121:50000 --> example.com:443 using DIRECT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SaveRecovery(RecoveryState{Stage: RecoveryGatewayActive, Required: true}); err != nil {
		t.Fatal(err)
	}
	response := performAuthorized(server, http.MethodPost, "/api/v1/recovery/client-validated", []byte(`{"client_ipv4":"192.168.1.121","gateway_dns_confirmed":true,"no_explicit_proxy_confirmed":true,"ipv6_bypass_warning_confirmed":false}`))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	state, _ := server.store.Recovery()
	if state.Stage != RecoveryClientValidated {
		t.Fatalf("state=%#v", state)
	}
}

func TestControlConfigCanInitializeDevicePolicyFile(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Runtime.Dir = filepath.Join(dir, "runtime")
	cfg.Mihomo.Config = filepath.Join(dir, "runtime", "mihomo.yaml")
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(config.Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}
	input := controlConfigFrom(cfg, fileDigest(path))
	input.DevicePolicy.Enabled = true
	payload, _ := json.Marshal(input)
	if _, err := applyControlConfig(path, input.Revision, payload); err != nil {
		t.Fatal(err)
	}
	updated, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DevicePolicy.File == "" {
		t.Fatal("device policy file was not initialized")
	}
	if _, err := os.Stat(updated.DevicePolicy.File); err != nil {
		t.Fatal(err)
	}
}

func TestDiagnosticLogTailRedactsKnownCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mihomo.log")
	if err := os.WriteFile(path, []byte("secret-token proxy-user proxy-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Mihomo.Secret = "secret-token"
	cfg.UpstreamProxy.Username = "proxy-user"
	cfg.UpstreamProxy.Password = "proxy-password"
	lines := tailLines(path, 10, cfg)
	if len(lines) != 1 || strings.Contains(lines[0], "secret") || strings.Contains(lines[0], "proxy-user") || strings.Contains(lines[0], "proxy-password") {
		t.Fatalf("redacted lines = %#v", lines)
	}
}

func TestDeviceTrafficKeepsLeaseInventoryWhenMihomoIsUnavailable(t *testing.T) {
	server := newTestServer(t)
	cfg, err := config.LoadRuntime(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	server.fetchConnections = func(context.Context, config.Config) (mihomo.ConnectionsSnapshot, error) {
		return mihomo.ConnectionsSnapshot{}, errors.New("mihomo unavailable")
	}
	paths := runtime.NewPaths(cfg)
	if err := os.MkdirAll(filepath.Dir(paths.LeaseFile), 0o700); err != nil {
		t.Fatal(err)
	}
	lease := fmt.Sprintf("%d aa:bb:cc:dd:ee:ff 192.168.1.151 iPhone-15 *\n", time.Now().Add(time.Hour).Unix())
	if err := os.WriteFile(paths.LeaseFile, []byte(lease), 0o600); err != nil {
		t.Fatal(err)
	}

	response := performAuthorized(server, http.MethodGet, "/api/v1/device-traffic", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("device traffic status=%d body=%s", response.Code, response.Body.String())
	}
	var payload DeviceTrafficResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Scope != deviceTrafficScope || len(payload.Devices) != 1 || payload.Devices[0].Hostname != "iPhone-15" {
		t.Fatalf("device traffic = %#v", payload)
	}
	if payload.ConnectionError == "" || payload.Totals.Devices != 1 || payload.Totals.ActiveConnections != 0 {
		t.Fatalf("unavailable mihomo response = %#v", payload)
	}
}

func TestDeviceTrafficEndpointAttributesLiveMihomoConnections(t *testing.T) {
	server := newTestServer(t)
	cfg, err := config.LoadRuntime(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	server.fetchConnections = func(context.Context, config.Config) (mihomo.ConnectionsSnapshot, error) {
		return mihomo.ConnectionsSnapshot{UploadTotal: 100, DownloadTotal: 900, Connections: []mihomo.Connection{
			{ID: "one", Upload: 100, Download: 900, Chains: []string{"流媒体组", "美国-02"}, Metadata: map[string]any{"sourceIP": "192.168.1.188"}},
		}}, nil
	}
	paths := runtime.NewPaths(cfg)
	if err := os.MkdirAll(filepath.Dir(paths.LeaseFile), 0o700); err != nil {
		t.Fatal(err)
	}
	lease := fmt.Sprintf("%d aa:bb:cc:dd:ee:88 192.168.1.188 Apple-TV *\n", time.Now().Add(time.Hour).Unix())
	if err := os.WriteFile(paths.LeaseFile, []byte(lease), 0o600); err != nil {
		t.Fatal(err)
	}

	response := performAuthorized(server, http.MethodGet, "/api/v1/device-traffic", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("device traffic status=%d body=%s", response.Code, response.Body.String())
	}
	var payload DeviceTrafficResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ConnectionError != "" || len(payload.Devices) != 1 {
		t.Fatalf("device traffic = %#v", payload)
	}
	device := payload.Devices[0]
	if device.ActiveConnections != 1 || device.Upload != 100 || device.Download != 900 || device.PrimaryEgress != "流媒体组 → 美国-02" {
		t.Fatalf("attributed device = %#v", device)
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
	policyPath := filepath.Join(dir, "device-policy.json")
	if err := os.WriteFile(policyPath, []byte(`{"devices":[],"profiles":[],"templates":[],"rule_sets":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`gateway:
  mode: "same_wifi_dhcp"
  interface: "en0"
  lan_ip: "192.168.1.20"
  upstream_interface: "en0"
dhcp:
  enabled: true
  range_start: "192.168.1.120"
  range_end: "192.168.1.199"
device_policy:
  file: "`+policyPath+`"
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
	server, err := New(Options{ConfigPath: configPath, Addr: "127.0.0.1:61767", StoreDir: filepath.Join(dir, "store"), Runner: fakeRunner{}, NetworkRunner: network, ConfigRunner: fakeConfigurationRunner{}, DiscoverNetwork: discover, PingRouter: func(context.Context, string) error { return nil }, Static: http.NotFoundHandler(), Credentials: &memoryCredentialStore{}})
	if err != nil {
		t.Fatal(err)
	}
	server.sessions["expired"] = time.Now().Add(-time.Minute)
	return server, network
}

type fakeRunner struct{}

func (fakeRunner) Run(_ context.Context, _, _ string) error { return nil }

type recordingActionRunner struct {
	action string
	count  int
}

func (r *recordingActionRunner) Run(_ context.Context, action, _ string) error {
	r.action = action
	r.count++
	return nil
}

type actionRunnerFunc func(context.Context, string, string) error

func (f actionRunnerFunc) Run(ctx context.Context, action, configPath string) error {
	return f(ctx, action, configPath)
}

func waitForStoredOperation(t *testing.T, server *Server, id, state string) Operation {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		op, err := server.store.Operation(id)
		if err == nil && op.State == state {
			return op
		}
		time.Sleep(10 * time.Millisecond)
	}
	op, err := server.store.Operation(id)
	t.Fatalf("operation %q did not reach %q: op=%#v err=%v", id, state, op, err)
	return Operation{}
}

type fakeConfigurationRunner struct {
	profileErr      error
	profileReloaded bool
}

func (f fakeConfigurationRunner) ApplyProfile(_ context.Context, _, revision string, _ []byte) (ProfileApplyResult, error) {
	if f.profileErr != nil {
		return ProfileApplyResult{}, f.profileErr
	}
	return ProfileApplyResult{Revision: revision + "-applied", Reloaded: f.profileReloaded}, nil
}

func (fakeConfigurationRunner) ApplyDevicePolicy(_ context.Context, _, _ string, payload []byte) (string, error) {
	var policy device.PolicySet
	if err := json.Unmarshal(payload, &policy); err != nil {
		return "", err
	}
	bundle, err := device.CompilePolicyBundle(policy)
	return bundle.Digest, err
}

func (fakeConfigurationRunner) ApplyControlConfig(_ context.Context, path, revision string, payload []byte) (string, error) {
	return applyControlConfig(path, revision, payload)
}

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
