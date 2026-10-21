package mihomo

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise the actual gateway engine starter with a delayed real core, using
// only a random loopback controller. No DNS, proxy, TUN or host takeover ports.
func TestStartupReadinessRealCore(t *testing.T) {
	binary := os.Getenv("OMG_PREPARED_MIHOMO_BINARY")
	if binary == "" {
		t.Skip("set OMG_PREPARED_MIHOMO_BINARY for the isolated delayed-start gate")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	quotedBinary := "'" + strings.ReplaceAll(binary, "'", "'\"'\"'") + "'"
	m := readinessManager(t, "http://"+addr, "sleep 2.5\nexec "+quotedBinary+" \"$@\"")
	m.cfg.Mihomo.Secret = "isolated-readiness-test"
	yaml := fmt.Sprintf("external-controller: %q\nsecret: %q\nport: 0\nsocks-port: 0\nmixed-port: 0\nredir-port: 0\ntproxy-port: 0\nallow-lan: false\nipv6: false\ndns: {enable: false}\ntun: {enable: false}\nrules: ['MATCH,DIRECT']\n", addr, m.cfg.Mihomo.Secret)
	if err := os.WriteFile(m.paths.MihomoConfig, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	pid, err := m.StartContext(t.Context())
	stopped := false
	if pid > 0 {
		t.Cleanup(func() {
			if !stopped {
				_ = m.Stop(pid)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 2500*time.Millisecond || elapsed > apiStartupTimeout+time.Second {
		t.Fatalf("unexpected delayed start duration: %s", elapsed)
	}
	version, err := FetchVersion(t.Context(), m.cfg)
	if err != nil || version.Version == "" {
		t.Fatalf("real controller unavailable: version=%+v err=%v", version, err)
	}
	t.Logf("delayed real core %s ready after %s", version.Version, time.Since(started).Round(time.Millisecond))
	if err := m.Stop(pid); err != nil {
		t.Fatal(err)
	}
	stopped = true
	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatal("real controller listener survived stop")
	}
}
