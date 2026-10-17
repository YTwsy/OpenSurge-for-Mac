package mihomo

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
)

// Inspect the actual core's parsed addresses without opening a TUN, DNS,
// packet listener or forwarding path. In particular, omitting inet6-address
// would restore Mihomo's default even when the host preference is disabled.
func TestMacTUNIPv6ConfigRealCore(t *testing.T) {
	binary := os.Getenv("OMG_PREPARED_MIHOMO_BINARY")
	if binary == "" {
		t.Skip("set OMG_PREPARED_MIHOMO_BINARY for the no-takeover core check")
	}
	for _, enabled := range []bool{true, false} {
		for _, dns := range []bool{true, false} {
			t.Run(fmt.Sprintf("host=%v/dns=%v", enabled, dns), func(t *testing.T) {
				cfg := config.Default()
				cfg.Transparent.Mode = config.TransparentModeTUN
				cfg.Transparent.MacTUNIPv6 = enabled
				cfg.DNS.IPv6 = dns
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				cfg.Mihomo.APIAddr = listener.Addr().String()
				listener.Close()
				cfg.Mihomo.Secret = "local-core-test"
				body, err := RenderConfig(cfg)
				if err != nil {
					t.Fatal(err)
				}
				var root map[string]any
				if err := yaml.Unmarshal([]byte(body), &root); err != nil {
					t.Fatal(err)
				}
				root["tun"].(map[string]any)["enable"] = false
				root["dns"].(map[string]any)["enable"] = false
				root["mixed-port"] = 0
				delete(root, "interface-name")
				data, err := yaml.Marshal(root)
				if err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yaml")
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				log, err := os.Create(filepath.Join(dir, "core.log"))
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(binary, "-d", dir, "-f", path)
				cmd.Stdout, cmd.Stderr = log, log
				if err := cmd.Start(); err != nil {
					log.Close()
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- cmd.Wait() }()
				t.Cleanup(func() {
					_ = cmd.Process.Signal(syscall.SIGTERM)
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						_ = cmd.Process.Kill()
						<-done
					}
					log.Close()
					if t.Failed() {
						data, _ := os.ReadFile(log.Name())
						t.Log(string(data))
					}
				})
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				for {
					state, err := FetchTUNRuntimeState(ctx, cfg)
					if err == nil {
						if state.Enabled {
							t.Fatal("test must not enable TUN")
						}
						// With DNS disabled and no active TUN the core can clear IPv6
						// on hosts without native IPv6. The opt-out must always be empty.
						if !enabled && len(state.IPv6Addresses) != 0 {
							t.Fatalf("core restored host IPv6: %v", state.IPv6Addresses)
						}
						if enabled && dns && (len(state.IPv6Addresses) != 1 || state.IPv6Addresses[0] != config.MihomoTUNIPv6) {
							t.Fatalf("host IPv6 lost: %v", state.IPv6Addresses)
						}
						return
					}
					if ctx.Err() != nil {
						t.Fatal(err)
					}
					time.Sleep(50 * time.Millisecond)
				}
			})
		}
	}
}
