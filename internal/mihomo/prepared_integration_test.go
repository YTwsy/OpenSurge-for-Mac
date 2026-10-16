package mihomo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

// This gate uses a real core with only an authenticated loopback controller
// and a local HTTP fixture. It never starts gateway.Manager, DNS listeners,
// TUN, pf, DHCP, forwarding, packet listeners or a real Tailscale identity.
func TestPreparedEngineRealCore(t *testing.T) {
	binary := os.Getenv("OMG_PREPARED_MIHOMO_BINARY")
	if binary == "" {
		t.Skip("set OMG_PREPARED_MIHOMO_BINARY to run the no-takeover real-core gate")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	var probes atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/provider.yaml" {
			w.Header().Set("Content-Type", "application/yaml")
			_, _ = io.WriteString(w, "proxies:\n  - {name: provider-node, type: direct}\n")
			return
		}
		probes.Add(1)
		// mihomo records delays in whole milliseconds; keep the local fixture
		// above zero so a very fast loopback response is not reported as empty.
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer origin.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		upstream, err := net.DialTimeout("tcp", origin.Listener.Addr().String(), time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer upstream.Close()
		client, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		go func() {
			_, _ = io.Copy(upstream, client)
			_ = upstream.Close()
		}()
		_, _ = io.Copy(client, upstream)
	}))
	defer proxy.Close()
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Binary = binary
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "gateway.yaml")
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = filepath.Join(cfg.Runtime.Dir, "effective", "profile.yaml")
	if err := os.MkdirAll(filepath.Dir(cfg.Mihomo.Profile), 0o700); err != nil {
		t.Fatal(err)
	}
	_, proxyPort, err := net.SplitHostPort(proxy.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	profile := fmt.Sprintf(`hosts:
  proxy.hosts.opensurge.test: 127.0.0.1
  alias.hosts.opensurge.test: proxy.hosts.opensurge.test
dns:
  use-hosts: true
  use-system-hosts: false
  nameserver: [udp://127.0.0.1:1]
proxies:
  - {name: extension-node, type: direct}
  - {name: hosts-node, type: http, server: alias.hosts.opensurge.test, port: %s}
proxy-providers:
  subscription:
    type: http
    url: %q
    path: ./provider-cache.yaml
    interval: 3600
    health-check: {enable: false}
proxy-groups:
  - {name: Manual, type: select, proxies: [extension-node, hosts-node, DIRECT, REJECT], use: [subscription]}
rules: ["MATCH,Manual"]
`, proxyPort, origin.URL+"/provider.yaml")
	if err := os.WriteFile(cfg.Mihomo.Profile, []byte(profile), 0o600); err != nil {
		t.Fatal(err)
	}
	final, err := RenderConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Mihomo.Config, []byte(final), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateConfigWithTimeout(3*time.Second, binary, filepath.Dir(cfg.Mihomo.Profile), cfg.Mihomo.Config); err != nil {
		t.Fatalf("final gateway config validation: %v", err)
	}
	defer func() {
		if err := StopPrepared(cfg); err != nil {
			t.Errorf("real core cleanup: %v", err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var first PreparedState
	if err := runtime.WithLifecycleLock(cfg, func() error {
		var err error
		first, err = PrepareLocked(ctx, cfg, final)
		if err != nil {
			return err
		}
		apiCfg := PreparedConfig(cfg, first)
		groups, err := FetchProxyGroups(ctx, apiCfg)
		if err != nil {
			return err
		}
		var manual ProxyGroup
		for _, group := range groups {
			if group.Name == "Manual" {
				manual = group
			}
		}
		if !containsPreparedOption(manual.Options, "extension-node") || !containsPreparedOption(manual.Options, "provider-node") {
			return fmt.Errorf("final/provider members unavailable: %+v", manual)
		}
		if err := SelectProxyGroup(ctx, apiCfg, "Manual", "provider-node"); err != nil {
			return err
		}
		result := MeasureProxyDelay(ctx, apiCfg, "extension-node", origin.URL+"/generate_204", 2*time.Second)
		if result.Status != "reachable" || probes.Load() == 0 {
			return fmt.Errorf("real local probe failed: %+v requests=%d", result, probes.Load())
		}
		// This proxy server can only be reached through the imported hosts
		// alias -> static IP chain; no DNS listener or usable upstream exists.
		beforeHostsProbe := probes.Load()
		result = MeasureProxyDelay(ctx, apiCfg, "hosts-node", origin.URL+"/generate_204", 2*time.Second)
		if result.Status != "reachable" || probes.Load() <= beforeHostsProbe {
			return fmt.Errorf("imported hosts node probe failed: %+v requests=%d", result, probes.Load())
		}
		if err := assertPreparedRuntimeNoIngress(ctx, apiCfg); err != nil {
			return err
		}
		// Validate-only is intentionally checked while the shared cache is
		// open. This evidence does not authorize concurrent configuration writes.
		if err := validateConfigWithTimeout(3*time.Second, binary, first.ConfigDirectory, filepath.Join(cfg.Runtime.Dir, preparedDirectoryName, "mihomo.yaml")); err != nil {
			return fmt.Errorf("same-directory validation beside prepared engine: %w", err)
		}
		if _, exists, err := runtime.LoadState(runtime.NewPaths(cfg).StateFile); err != nil || exists {
			return fmt.Errorf("prepared engine created gateway state: exists=%t err=%v", exists, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("lsof"); err == nil {
		out, err := exec.Command("lsof", "-nP", "-a", "-p", strconv.Itoa(first.PID), "-i").CombinedOutput()
		if err != nil {
			t.Fatalf("inspect prepared listeners: %v: %s", err, out)
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "(LISTEN)") && !strings.Contains(line, first.APIAddr) {
				t.Fatalf("unexpected core listener: %s", line)
			}
		}
	}
	if err := StopPrepared(cfg); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WithLifecycleLock(cfg, func() error {
		second, err := PrepareLocked(ctx, cfg, final)
		if err != nil {
			return err
		}
		if second.PID == first.PID || second.APIAddr == first.APIAddr || second.Secret == first.Secret {
			return fmt.Errorf("prepared restart did not renew private controller/process identity")
		}
		groups, err := FetchProxyGroups(ctx, PreparedConfig(cfg, second))
		if err != nil {
			return err
		}
		for _, group := range groups {
			if group.Name == "Manual" && group.Selected == "provider-node" {
				return nil
			}
		}
		return fmt.Errorf("shared cache did not restore selected provider node: %+v", groups)
	}); err != nil {
		t.Fatal(err)
	}
}

func containsPreparedOption(options []string, name string) bool {
	for _, option := range options {
		if option == name {
			return true
		}
	}
	return false
}

func assertPreparedRuntimeNoIngress(ctx context.Context, cfg config.Config) error {
	req, err := newAPIRequest(ctx, cfg, http.MethodGet, "/configs", nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var runtimeConfig map[string]any
	if err := json.NewDecoder(response.Body).Decode(&runtimeConfig); err != nil {
		return err
	}
	for _, key := range []string{"port", "socks-port", "mixed-port", "redir-port", "tproxy-port"} {
		if value, exists := runtimeConfig[key]; exists && value != float64(0) {
			return fmt.Errorf("runtime %s is not disabled: %v", key, value)
		}
	}
	if tun, ok := runtimeConfig["tun"].(map[string]any); !ok || tun["enable"] != false {
		return fmt.Errorf("runtime TUN is not disabled: %v", runtimeConfig["tun"])
	}
	return nil
}
