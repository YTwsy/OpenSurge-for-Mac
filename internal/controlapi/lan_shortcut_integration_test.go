package controlapi

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/dhcp"
	"open-mihomo-gateway/internal/runtime"
)

// Opt-in real dnsmasq, on isolated high loopback ports. No DHCP, root helper,
// interface changes, system DNS changes, PF or TUN are involved.
func TestLANShortcutRealDNSMasq(t *testing.T) {
	binary := os.Getenv("OMG_SHORTCUT_DNSMASQ_BINARY")
	if binary == "" {
		t.Skip("set OMG_SHORTCUT_DNSMASQ_BINARY for real dnsmasq forwarding")
	}
	var dnsAddress string
	listen := func(network, address string) (net.Listener, error) {
		l, err := net.Listen(network, "127.0.0.1:0")
		if err == nil && address == config.LANShortcutDNS {
			dnsAddress = l.Addr().String()
		}
		return l, err
	}
	packet := func(network, _ string) (net.PacketConn, error) { return net.ListenPacket(network, dnsAddress) }
	addresses := func() ([]lanAddress, error) {
		return []lanAddress{{Interface: "en0", IPv4: "192.168.50.1", PrefixLen: 22}}, nil
	}
	shortcut, err := startLANShortcut(context.Background(), shortcutTestBinding, listen, packet, addresses)
	if err != nil {
		t.Fatal(err)
	}
	defer shortcut.Close()
	reserve, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dnsmasqAddress := reserve.Addr().String()
	_, port, _ := net.SplitHostPort(dnsmasqAddress)
	reserve.Close()
	cfg := config.Default()
	cfg.DHCP.Enabled = false
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Off
	cfg.Gateway.Interface = "lo0"
	if goruntime.GOOS == "linux" {
		cfg.Gateway.Interface = "lo"
	}
	cfg.DNS.Listen = "127.0.0.1"
	cfg.DNS.Port, _ = strconv.Atoi(port)
	cfg.Runtime.Dir = t.TempDir()
	rendered, err := dhcp.RenderConfig(cfg, runtime.NewPaths(cfg))
	if err != nil {
		t.Fatal(err)
	}
	rendered = strings.Replace(rendered, "127.0.0.1#61769", strings.Replace(dnsAddress, ":", "#", 1), 1)
	configPath := filepath.Join(cfg.Runtime.Dir, "dnsmasq.conf")
	if err = os.WriteFile(configPath, []byte(rendered), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := exec.CommandContext(ctx, binary, "--keep-in-foreground", "--conf-file="+configPath, "--log-facility=-", "--pid-file=")
	var logs bytes.Buffer
	command.Stdout = &logs
	command.Stderr = &logs
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = command.Wait()
		if t.Failed() {
			t.Log(logs.String())
		}
	}()
	query := func(network string) ([]net.IP, error) {
		resolver := net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, dnsmasqAddress)
		}}
		queryCtx, queryCancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer queryCancel()
		return resolver.LookupIP(queryCtx, "ip4", "open.surge")
	}
	for _, network := range []string{"udp", "tcp"} {
		deadline := time.Now().Add(3 * time.Second)
		for {
			ips, err := query(network)
			if err == nil && len(ips) == 1 && ips[0].String() == shortcutTestBinding.IPv4 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal(fmt.Sprintf("%s dnsmasq forwarding: %v %v", network, ips, err))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	shortcut.Close()
	if ips, err := query("udp"); err == nil || len(ips) != 0 {
		t.Fatalf("dnsmasq cached a released shortcut: %v %v", ips, err)
	}
}
