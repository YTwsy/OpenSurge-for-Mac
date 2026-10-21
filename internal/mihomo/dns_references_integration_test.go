package mihomo

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"open-mihomo-gateway/internal/config"
)

// Uses real core processes and controlled loopback DNS/HTTP services, with fresh
// source/cache directories for every mode. The gateway-mode core uses the real
// 1053 binding but no gateway manager, TUN, DHCP, PF or system DNS takeover.
func TestDNSReferencesRealCore(t *testing.T) {
	binary := os.Getenv("OMG_PREPARED_MIHOMO_BINARY")
	if binary == "" {
		t.Skip("set OMG_PREPARED_MIHOMO_BINARY for the isolated real DNS gate")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, transport := range []struct {
		name, listener, target string
		policy                 bool
	}{
		{"IPv4-UDP", "0.0.0.0:7874", "udp://127.0.0.1:7874", false},
		{"IPv4-TCP", "0.0.0.0:7874", "tcp://127.0.0.1:7874", false},
		{"IPv6-UDP", "[::]:7874", "udp://[::1]:7874", false},
		{"IPv6-TCP", "[::]:7874", "tcp://[::1]:7874", false},
		{"node-policy", "0.0.0.0:7874", "udp://127.0.0.1:7874", true},
	} {
		for _, overlay := range []bool{false, true} {
			for _, mode := range []string{"prepared", "gateway-core"} {
				t.Run(fmt.Sprintf("%s/overlay=%t/%s", transport.name, overlay, mode), func(t *testing.T) {
					if mode == "gateway-core" && os.Getenv("OMG_DNS_REFERENCE_GATEWAY_CORE") != "1" {
						t.Skip("set OMG_DNS_REFERENCE_GATEWAY_CORE=1 only when the formal DNS port is available")
					}
					upstream, lookups := controlledDNSFixture(t)
					var hits atomic.Int32
					origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						hits.Add(1)
						time.Sleep(5 * time.Millisecond)
						if r.URL.Path == "/provider.yaml" {
							_, _ = io.WriteString(w, "proxies: [{name: loaded-provider-node, type: direct}]\n")
							return
						}
						w.WriteHeader(http.StatusNoContent)
					}))
					t.Cleanup(origin.Close)
					var connects atomic.Int32
					proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodConnect {
							http.Error(w, "CONNECT required", 405)
							return
						}
						remote, err := net.DialTimeout("tcp", origin.Listener.Addr().String(), time.Second)
						if err != nil {
							http.Error(w, err.Error(), 502)
							return
						}
						defer remote.Close()
						client, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							return
						}
						defer client.Close()
						connects.Add(1)
						_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
						go func() { _, _ = io.Copy(remote, client); _ = remote.Close() }()
						_, _ = io.Copy(client, remote)
					}))
					t.Cleanup(proxy.Close)
					_, proxyPort, _ := net.SplitHostPort(proxy.Listener.Addr().String())
					cfg := config.Default()
					cfg.Runtime.Dir = t.TempDir()
					cfg.LocalSystemDNS.Enabled = false
					cfg.Mihomo.Binary = binary
					cfg.Mihomo.MixedPort = 0
					cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "gateway.yaml")
					cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
					cfg.Mihomo.Profile = filepath.Join(cfg.Runtime.Dir, "source.yaml")
					cfg.Mihomo.APIAddr, cfg.Mihomo.Secret, err = newPreparedControllerIdentity()
					if err != nil {
						t.Fatal(err)
					}
					nodeDNS := fmt.Sprintf("  proxy-server-nameserver: [%q]", transport.target)
					if transport.policy {
						nodeDNS = fmt.Sprintf("  proxy-server-nameserver: [rcode://name_error]\n  proxy-server-nameserver-policy: {'+.node.opensurge.test': [%q]}", transport.target)
					}
					source := fmt.Sprintf(`hosts:
  original.node.opensurge.test: resolved.node.opensurge.test
dns:
  enable: true
  listen: %q
  use-hosts: true
  use-system-hosts: false
  nameserver: [%q]
%s
  fake-ip-filter: ['+.node.opensurge.test']
proxies:
  - {name: reference-node, type: http, server: original.node.opensurge.test, port: %s}
proxy-providers:
  controlled:
    type: http
    proxy: reference-node
    url: %q
    path: ./provider-cache.yaml
    interval: 3600
    health-check: {enable: false}
proxy-groups:
  - {name: Choice, type: select, proxies: [reference-node], use: [controlled]}
rules: ['MATCH,DIRECT']
`, transport.listener, "udp://"+upstream, nodeDNS, proxyPort, origin.URL+"/provider.yaml")
					doc := DefaultProfileOverlayDocument()
					doc.Enabled = overlay
					if overlay {
						doc.DNS.Append = map[string][]any{"fake-ip-filter": {"overlay.test"}}
					}
					composition, err := ComposeProfileOverlay([]byte(source), doc)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(cfg.Mihomo.Profile, []byte(composition.ProfileYAML), 0o600); err != nil {
						t.Fatal(err)
					}
					final, err := RenderConfig(cfg)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(cfg.Mihomo.Config, []byte(final), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := validateConfigWithTimeout(3*time.Second, binary, cfg.Runtime.Dir, cfg.Mihomo.Config); err != nil {
						t.Fatalf("formal generated configuration: %v", err)
					}
					ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
					defer cancel()
					apiCfg, endpoint := cfg, gatewayDNSEndpoint
					var prepared PreparedState
					if mode == "prepared" {
						prepared, err = Prepare(ctx, cfg, final)
						t.Cleanup(func() {
							if err := StopPrepared(cfg); err != nil {
								t.Error(err)
							}
						})
						if err != nil {
							logPreparedFailure(t, cfg)
							t.Fatal(err)
						}
						apiCfg, endpoint = PreparedConfig(cfg, prepared), prepared.DNSAddr
						again, err := Prepare(ctx, cfg, final)
						if err != nil || again.PID != prepared.PID || again.DNSAddr != prepared.DNSAddr {
							t.Fatalf("prepared DNS reuse: %+v %v", again, err)
						}
					} else {
						// Refuse to run if any existing gateway owns either transport.
						tcp, err := net.Listen("tcp4", "0.0.0.0:1053")
						if err != nil {
							t.Fatalf("formal DNS port occupied; existing service preserved: %v", err)
						}
						udp, err := net.ListenPacket("udp4", "0.0.0.0:1053")
						if err != nil {
							tcp.Close()
							t.Fatalf("formal UDP DNS port occupied; existing service preserved: %v", err)
						}
						tcp.Close()
						udp.Close()
						startDNSGatewayCore(t, ctx, cfg, final)
					}
					before := connects.Load()
					probe := MeasureProxyDelay(ctx, apiCfg, "reference-node", origin.URL+"/generate_204", 3*time.Second)
					if probe.Status != "reachable" || connects.Load() <= before || lookups.Load() == 0 {
						logPreparedFailure(t, cfg)
						t.Fatalf("node did not use alias + controlled DNS + proxy: %+v DNS=%d CONNECT=%d", probe, lookups.Load(), connects.Load())
					}
					found := false
					for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); {
						groups, err := FetchProxyGroups(ctx, apiCfg)
						if err != nil {
							t.Fatal(err)
						}
						for _, group := range groups {
							found = found || containsPreparedOption(group.Options, "loaded-provider-node")
						}
						if found {
							break
						}
						time.Sleep(50 * time.Millisecond)
					}
					if !found || hits.Load() < 2 {
						t.Fatal("provider did not load through the DNS-dependent node")
					}
					// Ordinary business DNS must still return Fake-IP; the node alias
					// filter must return the controlled real address over BOTH transports.
					for _, network := range []string{"udp", "tcp"} {
						answer := queryFixtureA(t, network, endpoint, "original.node.opensurge.test")
						if answer != [4]byte{127, 0, 0, 1} {
							t.Fatalf("node DNS returned %v", answer)
						}
						fake := queryFixtureA(t, network, endpoint, "business.opensurge.test")
						if fake[0] != 198 || fake[1] != 18 {
							t.Fatalf("business Fake-IP changed: %v", fake)
						}
					}
					if mode == "prepared" {
						if err := assertPreparedRuntimeNoIngress(ctx, apiCfg); err != nil {
							t.Fatal(err)
						}
						if err := StopPrepared(cfg); err != nil {
							t.Fatal(err)
						}
						assertDNSPortReleased(t, endpoint)
					}
				})
			}
		}
	}
}

func logPreparedFailure(t *testing.T, cfg config.Config) {
	t.Helper()
	for _, file := range []string{filepath.Join(cfg.Runtime.Dir, preparedDirectoryName, "mihomo.log"), filepath.Join(cfg.Runtime.Dir, "core.log")} {
		if data, err := os.ReadFile(file); err == nil {
			t.Log(string(data))
		}
	}
}

func startDNSGatewayCore(t *testing.T, ctx context.Context, cfg config.Config, final string) {
	t.Helper()
	if err := os.WriteFile(cfg.Mihomo.Config, []byte(final), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateConfigWithTimeout(3*time.Second, cfg.Mihomo.Binary, cfg.Runtime.Dir, cfg.Mihomo.Config); err != nil {
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
	})
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); {
		if _, err := FetchVersion(ctx, cfg); err == nil {
			probeName, err := finalDNSProbeName(final)
			if err != nil {
				t.Fatal(err)
			}
			if err := (Manager{}).waitForDNS(t.Context(), cmd.Process.Pid, probeName, 4*time.Second); err != nil {
				logPreparedFailure(t, cfg)
				t.Fatal(err)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	logPreparedFailure(t, cfg)
	t.Fatal("formal core controller and DNS listeners not ready")
}

func controlledDNSFixture(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	tcp, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udp, err := net.ListenPacket("udp4", tcp.Addr().String())
	if err != nil {
		tcp.Close()
		t.Fatal(err)
	}
	count := &atomic.Int32{}
	answer := func(data []byte) []byte {
		var request dnsmessage.Message
		if request.Unpack(data) != nil || len(request.Questions) != 1 {
			return nil
		}
		q := request.Questions[0]
		response := dnsmessage.Message{Header: dnsmessage.Header{ID: request.ID, Response: true, RecursionAvailable: true, RCode: dnsmessage.RCodeNameError}, Questions: request.Questions}
		// No answer for original.node: only the imported hosts alias can reach
		// this name. Neither the OS resolver nor another client can supply it.
		if q.Name.String() == "resolved.node.opensurge.test." {
			count.Add(1)
			response.RCode = dnsmessage.RCodeSuccess
			if q.Type == dnsmessage.TypeA {
				response.Answers = []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 1}, Body: &dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}}}}
			}
		}
		result, _ := response.Pack()
		return result
	}
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, peer, err := udp.ReadFrom(buffer)
			if err != nil {
				return
			}
			_, _ = udp.WriteTo(answer(buffer[:n]), peer)
		}
	}()
	go func() {
		for {
			conn, err := tcp.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				for {
					var length [2]byte
					if _, err := io.ReadFull(conn, length[:]); err != nil {
						return
					}
					data := make([]byte, binary.BigEndian.Uint16(length[:]))
					if _, err := io.ReadFull(conn, data); err != nil {
						return
					}
					result := answer(data)
					_, _ = conn.Write(append([]byte{byte(len(result) >> 8), byte(len(result))}, result...))
				}
			}()
		}
	}()
	t.Cleanup(func() { tcp.Close(); udp.Close() })
	return tcp.Addr().String(), count
}

func queryFixtureA(t *testing.T, network, endpoint, domain string) [4]byte {
	t.Helper()
	name, _ := dnsmessage.NewName(domain + ".")
	request := dnsmessage.Message{Header: dnsmessage.Header{ID: 123, RecursionDesired: true}, Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}}}
	data, err := request.Pack()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout(network, endpoint, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if network == "tcp" {
		data = append([]byte{byte(len(data) >> 8), byte(len(data))}, data...)
	}
	if _, err := conn.Write(data); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4096)
	if network == "tcp" {
		var length [2]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			t.Fatal(err)
		}
		buffer = make([]byte, binary.BigEndian.Uint16(length[:]))
		if _, err := io.ReadFull(conn, buffer); err != nil {
			t.Fatal(err)
		}
	} else {
		n, err := conn.Read(buffer)
		if err != nil {
			t.Fatal(err)
		}
		buffer = buffer[:n]
	}
	var response dnsmessage.Message
	if err := response.Unpack(buffer); err != nil {
		t.Fatal(err)
	}
	for _, record := range response.Answers {
		if a, ok := record.Body.(*dnsmessage.AResource); ok {
			return a.A
		}
	}
	t.Fatalf("no A answer for %s: %+v", domain, response)
	return [4]byte{}
}

func assertDNSPortReleased(t *testing.T, endpoint string) {
	t.Helper()
	tcp, err := net.Listen("tcp4", endpoint)
	if err != nil {
		t.Fatalf("TCP DNS listener leaked: %v", err)
	}
	tcp.Close()
	udp, err := net.ListenPacket("udp4", endpoint)
	if err != nil {
		t.Fatalf("UDP DNS listener leaked: %v", err)
	}
	udp.Close()
}

func TestPreparedDNSBindRaceRealCore(t *testing.T) {
	binary := os.Getenv("OMG_PREPARED_MIHOMO_BINARY")
	if binary == "" {
		t.Skip("set OMG_PREPARED_MIHOMO_BINARY for real bind-race recovery")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, network := range []string{"udp4", "tcp4"} {
		t.Run(network, func(t *testing.T) {
			cfg := config.Default()
			cfg.Runtime.Dir = t.TempDir()
			cfg.Mihomo.Binary = binary
			cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "gateway.yaml")
			final, err := renderDNSFixture(t, selfDNSFixture)
			if err != nil {
				t.Fatal(err)
			}
			deps := defaultPreparedDeps()
			deps.readyTimeout = 2 * time.Second
			allocations := 0
			conflicted := ""
			deps.reserveDNS = func() (string, func(), error) {
				address, release, err := reservePreparedDNS()
				if err != nil {
					return "", nil, err
				}
				allocations++
				if allocations != 1 {
					return address, release, nil
				}
				conflicted = address
				var once sync.Once
				return address, func() {
					once.Do(func() {
						release()
						var occupied io.Closer
						var err error
						if network == "udp4" {
							occupied, err = net.ListenPacket(network, address)
						} else {
							occupied, err = net.Listen(network, address)
						}
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { occupied.Close() })
					})
				}, nil
			}
			t.Cleanup(func() {
				if err := StopPrepared(cfg); err != nil {
					t.Error(err)
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			state, err := prepareLocked(ctx, cfg, final, deps)
			if err != nil {
				logPreparedFailure(t, cfg)
				t.Fatal(err)
			}
			if allocations != 2 || state.DNSAddr == conflicted {
				t.Fatalf("bind race was not recovered sequentially: attempts=%d addr=%s", allocations, state.DNSAddr)
			}
			if err := StopPrepared(cfg); err != nil {
				t.Fatal(err)
			}
			assertDNSPortReleased(t, state.DNSAddr)
		})
	}
}
