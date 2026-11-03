package mihomo

import (
	"bufio"
	"context"
	"crypto/tls"
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
	"syscall"
	"testing"
	"time"

	"golang.org/x/net/proxy"
	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
)

// The client gives SOCKS5 only a literal IP, then sends HTTP Host or TLS SNI.
// Controlled CONNECT outbounds expose both the chosen rule and the forwarded
// destination. All listeners are loopback: no external DNS, TUN, PF or DHCP.
func TestSnifferRealCore(t *testing.T) {
	binary := os.Getenv("OMG_PREPARED_MIHOMO_BINARY")
	if binary == "" {
		t.Skip("set OMG_PREPARED_MIHOMO_BINARY for the isolated sniffer gate")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	originHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "sniffer-origin-ok\n")
		w.(http.Flusher).Flush()
		// Keep the session observable until the client has checked /connections.
		<-r.Context().Done()
	})
	plain := httptest.NewServer(originHandler)
	defer plain.Close()
	secure := httptest.NewTLSServer(originHandler)
	defer secure.Close()
	type forwarded struct{ route, address string }
	hits := make(chan forwarded, 16)
	newOutbound := func(route string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodConnect {
				http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
				return
			}
			_, port, err := net.SplitHostPort(r.Host)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			origin := secure.Listener.Addr().String()
			if port == "80" || port == "8080" {
				origin = plain.Listener.Addr().String()
			}
			remote, err := net.DialTimeout("tcp", origin, time.Second)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer remote.Close()
			client, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer client.Close()
			_ = remote.SetDeadline(time.Now().Add(10 * time.Second))
			_ = client.SetDeadline(time.Now().Add(10 * time.Second))
			hits <- forwarded{route, r.Host}
			_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
			go func() { _, _ = io.Copy(remote, client); _ = remote.Close() }()
			_, _ = io.Copy(client, remote)
		}))
	}
	domainRoute := newOutbound("domain-route")
	defer domainRoute.Close()
	fallbackRoute := newOutbound("fallback-route")
	defer fallbackRoute.Close()
	_, domainPort, _ := net.SplitHostPort(domainRoute.Listener.Addr().String())
	_, fallbackPort, _ := net.SplitHostPort(fallbackRoute.Listener.Addr().String())

	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			cfg := config.Default()
			cfg.Runtime.Dir = t.TempDir()
			cfg.Mihomo.Binary = binary
			cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
			cfg.Mihomo.SnifferEnabled = enabled
			cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
			cfg.Mihomo.Profile = filepath.Join(cfg.Runtime.Dir, "profile.yaml")
			cfg.Mihomo.APIAddr, cfg.Mihomo.Secret, err = newPreparedControllerIdentity()
			if err != nil {
				t.Fatal(err)
			}
			mixedAddr, _, err := newPreparedControllerIdentity()
			if err != nil {
				t.Fatal(err)
			}
			_, mixedPort, _ := net.SplitHostPort(mixedAddr)
			cfg.Mihomo.MixedPort, err = strconv.Atoi(mixedPort)
			if err != nil {
				t.Fatal(err)
			}
			profile := fmt.Sprintf(`proxies:
  - {name: domain-route, type: http, server: 127.0.0.1, port: %s}
  - {name: fallback-route, type: http, server: 127.0.0.1, port: %s}
rules:
  - DOMAIN,example.com,domain-route
  - MATCH,fallback-route
`, domainPort, fallbackPort)
			if err := os.WriteFile(cfg.Mihomo.Profile, []byte(profile), 0o600); err != nil {
				t.Fatal(err)
			}
			final, err := RenderConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			// Keep the generated sniffer/rules intact; only isolate listeners.
			var root map[string]any
			if err := yaml.Unmarshal([]byte(final), &root); err != nil {
				t.Fatal(err)
			}
			root["dns"] = map[string]any{"enable": false}
			root["allow-lan"], root["bind-address"] = false, "127.0.0.1"
			isolated, err := yaml.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			startSnifferCore(t, cfg, isolated)
			dialer, err := proxy.SOCKS5("tcp", mixedAddr, nil, &net.Dialer{Timeout: 3 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			for _, probe := range []struct {
				name, port string
				tls, sniff bool
			}{
				{"HTTP", "80", false, true},
				{"HTTP-range", "8080", false, true},
				{"TLS", "443", true, true},
				{"TLS-alternate", "8443", true, true},
				{"outside-sniff-ports", "9443", true, false},
			} {
				t.Run(probe.name, func(t *testing.T) {
					target := net.JoinHostPort("198.51.100.7", probe.port)
					conn, err := dialer.Dial("tcp", target)
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
					if probe.tls {
						// httptest's certificate covers example.com; keep verification.
						tlsConfig := secure.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
						tlsConfig.ServerName = "example.com"
						conn = tls.Client(conn, tlsConfig)
						defer conn.Close()
					}
					if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"); err != nil {
						t.Fatal(err)
					}
					response, err := http.ReadResponse(bufio.NewReader(conn), nil)
					if err != nil {
						t.Fatal(err)
					}
					if response.StatusCode != http.StatusOK {
						t.Fatalf("origin status = %d", response.StatusCode)
					}
					body := make([]byte, len("sniffer-origin-ok\n"))
					if _, err := io.ReadFull(response.Body, body); err != nil || string(body) != "sniffer-origin-ok\n" {
						t.Fatalf("origin body = %q, err = %v", body, err)
					}
					wantRoute, wantRule, wantSniff := "fallback-route", "Match", ""
					if enabled && probe.sniff {
						wantRoute, wantRule, wantSniff = "domain-route", "Domain", "example.com"
					}
					select {
					case hit := <-hits:
						if hit.route != wantRoute || hit.address != target {
							t.Fatalf("forwarded = %+v, want route=%s unchanged destination=%s", hit, wantRoute, target)
						}
					case <-time.After(time.Second):
						t.Fatal("controlled outbound was not used")
					}
					connections, err := FetchConnections(t.Context(), cfg)
					if err != nil {
						t.Fatal(err)
					}
					for _, connection := range connections.Connections {
						if fmt.Sprint(connection.Metadata["destinationPort"]) != probe.port {
							continue
						}
						sniff, _ := connection.Metadata["sniffHost"].(string)
						if connection.Rule != wantRule || sniff != wantSniff || connection.Metadata["destinationIP"] != "198.51.100.7" {
							t.Fatalf("unexpected routing metadata: %+v", connection)
						}
						if wantSniff != "" && connection.RulePayload != wantSniff {
							t.Fatalf("domain rule payload = %q", connection.RulePayload)
						}
						return
					}
					t.Fatalf("probe missing from active connections: %+v", connections)
				})
			}
		})
	}
}

func startSnifferCore(t *testing.T, cfg config.Config, content []byte) {
	t.Helper()
	if err := os.WriteFile(cfg.Mihomo.Config, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateConfigWithTimeout(5*time.Second, cfg.Mihomo.Binary, cfg.Runtime.Dir, cfg.Mihomo.Config); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(cfg.Runtime.Dir, "core.log"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(cfg.Mihomo.Binary, "-d", cfg.Runtime.Dir, "-f", cfg.Mihomo.Config)
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
			logPreparedFailure(t, cfg)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		if _, err := FetchVersion(ctx, cfg); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("isolated sniffer core was not ready: %s", strings.TrimSpace(cfg.Mihomo.APIAddr))
}
