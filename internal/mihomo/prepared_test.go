package mihomo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

func TestRenderPreparedConfigKeepsFinalPolicyWithoutIngress(t *testing.T) {
	final := `mixed-port: 17890
port: 8080
socks-port: 1080
redir-port: 7892
tproxy-port: 7893
allow-lan: true
interface-name: en0
external-controller: 0.0.0.0:9090
external-controller-unix: /tmp/unsafe-controller.sock
external-controller-tls: 0.0.0.0:9443
external-ui: /tmp/ui
secret: gateway-secret
mode: rule
ntp: {enable: true, write-to-system: true}
tun: {enable: true, auto-route: true, auto-redirect: true}
listeners: [{name: packet, type: opensurge-packet, socket: /tmp/packet.sock}]
tunnels: [{network: [tcp, udp], address: 0.0.0.0:2222, target: example.com:443}]
profile: {store-selected: true, store-fake-ip: true}
proxies: [{name: extra-node, type: direct}]
proxy-providers:
  subscription: {type: file, path: /tmp/provider.yaml}
proxy-groups:
  - {name: Manual, type: select, proxies: [extra-node, DIRECT], use: [subscription]}
rule-providers:
  local: {type: file, behavior: classical, path: /tmp/rules.yaml}
rules: ["RULE-SET,local,Manual", "MATCH,Manual"]
dns:
  enable: true
  listen: 0.0.0.0:1053
  nameserver: [1.1.1.1]
  proxy-server-nameserver: [9.9.9.9]
  nameserver-policy: {"+.internal": 10.0.0.1}
`
	rendered, err := RenderPreparedConfig(final, "127.0.0.1:19090", "private-secret")
	if err != nil {
		t.Fatal(err)
	}
	var original, prepared map[string]any
	if err := yaml.Unmarshal([]byte(final), &original); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(rendered), &prepared); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"proxies", "proxy-providers", "proxy-groups", "rules", "rule-providers", "profile"} {
		if !reflect.DeepEqual(original[key], prepared[key]) {
			t.Errorf("final %s changed: %#v", key, prepared[key])
		}
	}
	for _, key := range []string{"interface-name", "ntp", "external-ui", "external-controller-unix", "external-controller-tls"} {
		if _, exists := prepared[key]; exists {
			t.Errorf("unsafe field %s retained", key)
		}
	}
	for _, key := range []string{"port", "socks-port", "mixed-port", "redir-port", "tproxy-port"} {
		if prepared[key] != 0 {
			t.Errorf("business listener %s = %#v", key, prepared[key])
		}
	}
	if prepared["allow-lan"] != false || prepared["bind-address"] != "127.0.0.1" || prepared["external-controller"] != "127.0.0.1:19090" || prepared["secret"] != "private-secret" {
		t.Fatal("prepared controller boundary incorrect")
	}
	if tun := prepared["tun"].(map[string]any); tun["enable"] != false || tun["auto-route"] != false {
		t.Fatalf("TUN remains enabled: %#v", tun)
	}
	if len(prepared["listeners"].([]any)) != 0 || len(prepared["tunnels"].([]any)) != 0 {
		t.Fatal("custom ingress remains enabled")
	}
	dns := prepared["dns"].(map[string]any)
	if dns["listen"] != "" || dns["enable"] != true {
		t.Fatalf("resolver/listener boundary = %#v", dns)
	}
	for _, key := range []string{"nameserver", "proxy-server-nameserver", "nameserver-policy"} {
		if !reflect.DeepEqual(original["dns"].(map[string]any)[key], dns[key]) {
			t.Errorf("DNS resolver setting %s changed", key)
		}
	}
}

func TestRenderPreparedConfigRejectsUnauthenticatedOrRemoteController(t *testing.T) {
	for _, tc := range []struct{ address, secret string }{{"0.0.0.0:9090", "secret"}, {"127.0.0.1:9090", ""}, {"https://127.0.0.1:9090", "secret"}} {
		if _, err := RenderPreparedConfig("rules: [MATCH,DIRECT]", tc.address, tc.secret); err == nil {
			t.Errorf("accepted %q with secret %q", tc.address, tc.secret)
		}
	}
}

type fakePreparedEngine struct {
	starts     int
	stops      int
	alive      bool
	stopErr    error
	workDir    string
	boot       runtime.BootSession
	versionErr error
}

func preparedFixture(t *testing.T) (config.Config, *fakePreparedEngine, preparedDeps) {
	t.Helper()
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "gateway.yaml")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Mihomo.Binary = binary
	fake := &fakePreparedEngine{boot: runtime.BootSession{ID: "current-boot", StartedAt: time.Now().Add(-time.Hour)}}
	deps := preparedDeps{
		boot:        func() (runtime.BootSession, error) { return fake.boot, nil },
		fingerprint: func(int) (string, error) { return "owned-process", nil },
		matches: func(pid int, fingerprint string) (bool, error) {
			return fake.alive && pid == 777 && fingerprint == "owned-process", nil
		},
		start: func(logPath, binary, directory, path string) (int, error) {
			journal, exists, err := LoadPrepared(cfg)
			if err != nil || !exists || journal.PID != 0 {
				t.Fatalf("missing pre-fork journal: %+v %t %v", journal, exists, err)
			}
			fake.starts++
			fake.alive = true
			fake.workDir = directory
			return 777, nil
		},
		stop: func(int, time.Duration) error {
			fake.stops++
			if fake.stopErr == nil {
				fake.alive = false
			}
			return fake.stopErr
		},
		validate: func(string, string, string) error { return nil },
		version: func(context.Context, config.Config) (Version, error) {
			return Version{Version: "test"}, fake.versionErr
		},
	}
	return cfg, fake, deps
}

func TestPrepareReusesEngineAndHandsOffSharedCache(t *testing.T) {
	cfg, fake, deps := preparedFixture(t)
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = filepath.Join(cfg.Runtime.Dir, "effective", "profile.yaml")
	final := "proxy-groups: [{name: Manual, type: select, proxies: [DIRECT, REJECT]}]\nrules: [\"MATCH,Manual\"]\n"
	first, err := prepareLocked(context.Background(), cfg, final, deps)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareLocked(context.Background(), cfg, final, deps)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || fake.starts != 1 || fake.stops != 0 {
		t.Fatalf("same final graph replaced its live engine: %d starts %d stops", fake.starts, fake.stops)
	}
	if fake.workDir != filepath.Dir(cfg.Mihomo.Profile) {
		t.Fatalf("cache directory %q does not match gateway %q", fake.workDir, filepath.Dir(cfg.Mihomo.Profile))
	}
	for _, path := range []string{preparedStatePath(cfg), filepath.Join(cfg.Runtime.Dir, preparedDirectoryName, "mihomo.yaml"), filepath.Join(cfg.Runtime.Dir, preparedDirectoryName, "mihomo.log")} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("private artifact %q mode/error: %v %v", path, info, err)
		}
	}
	if _, err := prepareLocked(context.Background(), cfg, final+"log-level: warning\n", deps); err != nil {
		t.Fatal(err)
	}
	if fake.starts != 2 || fake.stops != 1 {
		t.Fatalf("changed graph did not replace sequentially: %d starts %d stops", fake.starts, fake.stops)
	}
	if err := stopPreparedLocked(cfg, deps); err != nil {
		t.Fatal(err)
	}
	if _, exists, err := LoadPrepared(cfg); err != nil || exists {
		t.Fatalf("stop retained state: %t %v", exists, err)
	}
	if _, err := os.Stat(fake.workDir); err != nil {
		t.Fatalf("stop deleted durable cache directory: %v", err)
	}
}

func TestPrepareRefusesGatewayStateAndHeldGatewayLock(t *testing.T) {
	cfg, fake, deps := preparedFixture(t)
	if err := runtime.SaveState(runtime.NewPaths(cfg).StateFile, runtime.State{BootSessionID: "previous-boot"}); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareLocked(context.Background(), cfg, "rules: [\"MATCH,DIRECT\"]", deps); !errors.Is(err, ErrGatewayOwnsEngine) || fake.starts != 0 {
		t.Fatalf("prepared while gateway state exists: %v", err)
	}
	if err := runtime.WithLifecycleLock(cfg, func() error {
		_, err := Prepare(context.Background(), cfg, "rules: [\"MATCH,DIRECT\"]")
		if !errors.Is(err, runtime.ErrLifecycleOperationInProgress) {
			t.Fatalf("Prepare bypassed gateway lock: %v", err)
		}
		if err := StopPrepared(cfg); !errors.Is(err, runtime.ErrLifecycleOperationInProgress) {
			t.Fatalf("StopPrepared bypassed gateway lock: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedCleanupNeverSignalsOldBootOrReusedPID(t *testing.T) {
	for _, scenario := range []string{"old-boot", "reused-pid"} {
		t.Run(scenario, func(t *testing.T) {
			cfg, fake, deps := preparedFixture(t)
			state, err := prepareLocked(context.Background(), cfg, "rules: [\"MATCH,DIRECT\"]", deps)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "old-boot" {
				fake.boot.ID = "next-boot"
			} else {
				state.ProcessFingerprint = "other-process"
				if err := savePrepared(cfg, state); err != nil {
					t.Fatal(err)
				}
			}
			if err := stopPreparedLocked(cfg, deps); err != nil {
				t.Fatal(err)
			}
			if fake.stops != 0 {
				t.Fatal("signalled an unowned process")
			}
		})
	}
}

func TestPreparedCleanupFailureRetainsOwnershipAndBlocksReplacement(t *testing.T) {
	cfg, fake, deps := preparedFixture(t)
	if _, err := prepareLocked(context.Background(), cfg, "rules: [\"MATCH,DIRECT\"]", deps); err != nil {
		t.Fatal(err)
	}
	fake.stopErr = errors.New("permission denied")
	if _, err := prepareLocked(context.Background(), cfg, "rules: [\"MATCH,REJECT\"]", deps); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("replacement ignored stop failure: %v", err)
	}
	if fake.starts != 1 {
		t.Fatal("started competing engine")
	}
	if _, exists, err := LoadPrepared(cfg); err != nil || !exists {
		t.Fatalf("lost recovery record: exists=%t err=%v", exists, err)
	}
}

func TestPreparedIncompleteIdentityFailsClosed(t *testing.T) {
	cfg, fake, deps := preparedFixture(t)
	state, err := prepareLocked(context.Background(), cfg, "rules: [\"MATCH,DIRECT\"]", deps)
	if err != nil {
		t.Fatal(err)
	}
	state.PID = 0
	state.ProcessFingerprint = ""
	if err := savePrepared(cfg, state); err != nil {
		t.Fatal(err)
	}
	if err := stopPreparedLocked(cfg, deps); err == nil || !strings.Contains(err.Error(), "lacks a process identity") {
		t.Fatalf("incomplete identity cleanup = %v", err)
	}
	if fake.stops != 0 {
		t.Fatal("signalled journal PID without fingerprint")
	}
	if _, err := prepareLocked(t.Context(), cfg, "rules: [\"MATCH,REJECT\"]", deps); err == nil || !strings.Contains(err.Error(), "lacks a process identity") {
		t.Fatalf("replacement bypassed incomplete identity: %v", err)
	}
	if fake.starts != 1 || fake.stops != 0 {
		t.Fatal("incomplete journal allowed a competing engine or an untracked signal")
	}
	if _, exists, err := LoadPrepared(cfg); err != nil || !exists {
		t.Fatalf("incomplete recovery journal was discarded: exists=%t, %v", exists, err)
	}
}

func TestPreparedCorruptJournalBlocksEngineChangesWithoutDiscardingEvidence(t *testing.T) {
	cfg, fake, deps := preparedFixture(t)
	path := preparedStatePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	broken := []byte("{interrupted-journal")
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := stopPreparedLocked(cfg, deps); err == nil {
		t.Fatal("cleanup accepted an unreadable journal")
	}
	if _, err := prepareLocked(t.Context(), cfg, "rules: [\"MATCH,DIRECT\"]", deps); err == nil {
		t.Fatal("replacement accepted an unreadable journal")
	}
	if fake.starts != 0 || fake.stops != 0 {
		t.Fatal("unreadable journal caused process changes")
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != string(broken) {
		t.Fatalf("unreadable recovery journal was discarded: %q, %v", content, err)
	}
}

func TestPreparedControllerFailureDoesNotAutomaticallyReplaceLiveEngine(t *testing.T) {
	cfg, fake, deps := preparedFixture(t)
	final := "rules: [\"MATCH,DIRECT\"]"
	if _, err := prepareLocked(context.Background(), cfg, final, deps); err != nil {
		t.Fatal(err)
	}
	fake.versionErr = fmt.Errorf("controller unavailable")
	if _, err := prepareLocked(context.Background(), cfg, final, deps); err == nil {
		t.Fatal("unavailable controller succeeded")
	}
	if fake.starts != 1 || fake.stops != 0 {
		t.Fatal("API failure triggered an implicit process replacement")
	}
}
