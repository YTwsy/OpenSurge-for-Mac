package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"open-mihomo-gateway/internal/config"
)

var shortcutTestBinding = lanSettings{Enabled: true, Interface: "en0", IPv4: "192.168.50.1", PrefixLen: 22, Port: 61768}

func TestLANShortcutRedirectBoundary(t *testing.T) {
	live := true
	handler := lanShortcutHandler(shortcutTestBinding, func() bool { return live })
	cases := []struct {
		name, method, host, path, peer string
		code                           int
	}{
		{"root", "GET", "open.surge", "/", "192.168.51.102:1234", 302},
		{"head", "HEAD", "OPEN.SURGE:80", "/", "192.168.50.2:1234", 302},
		{"query is discarded", "GET", "open.surge", "/?next=https://evil.example&code=secret", "192.168.50.2:1234", 302},
		{"other host", "GET", "evil.example", "/", "192.168.50.2:1234", 403},
		{"port", "GET", "open.surge:8080", "/", "192.168.50.2:1234", 403},
		{"outside subnet", "GET", "open.surge", "/", "192.168.54.2:1234", 403},
		{"loopback is not a LAN peer", "GET", "open.surge", "/", "127.0.0.1:1234", 403},
		{"API", "GET", "open.surge", "/api/v1/overview", "192.168.50.2:1234", 404},
		{"POST", "POST", "open.surge", "/", "192.168.50.2:1234", 405},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://"+tt.host+tt.path, nil)
			req.RemoteAddr = tt.peer
			req.Header.Set("X-Forwarded-For", "192.168.50.2")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != tt.code {
				t.Fatalf("status %d: %s", rr.Code, rr.Body)
			}
			if rr.Header().Get("Cache-Control") != "no-store" || len(rr.Result().Cookies()) != 0 {
				t.Fatal("cache/session boundary")
			}
			if tt.code == 302 && rr.Header().Get("Location") != "http://192.168.50.1:61768/" {
				t.Fatal(rr.Header())
			}
		})
	}
	live = false
	req := httptest.NewRequest("GET", "http://open.surge/", nil)
	req.RemoteAddr = "192.168.50.2:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 503 {
		t.Fatal("stale request was redirected")
	}
}

func TestLANShortcutDNSAuthority(t *testing.T) {
	for _, tt := range []struct {
		name    string
		typ     dnsmessage.Type
		live    bool
		code    dnsmessage.RCode
		answers int
	}{
		{"open.surge.", dnsmessage.TypeA, true, 0, 1},
		{"OPEN.SURGE.", dnsmessage.TypeAAAA, true, 0, 0},
		{"open.surge.", dnsmessage.Type(65), true, 0, 0},
		{"open.surge.", dnsmessage.TypeA, false, dnsmessage.RCodeNameError, 0},
		{"other.example.", dnsmessage.TypeA, true, dnsmessage.RCodeRefused, 0},
		{"sub.open.surge.", dnsmessage.TypeA, true, dnsmessage.RCodeRefused, 0},
	} {
		name, _ := dnsmessage.NewName(tt.name)
		q := dnsmessage.Message{Header: dnsmessage.Header{ID: 42}, Questions: []dnsmessage.Question{{Name: name, Type: tt.typ, Class: dnsmessage.ClassINET}}}
		data, _ := q.Pack()
		var reply dnsmessage.Message
		if err := reply.Unpack(lanShortcutDNSReply(data, shortcutTestBinding, tt.live)); err != nil {
			t.Fatal(err)
		}
		if reply.ID != 42 || !reply.Response || reply.RCode != tt.code || len(reply.Answers) != tt.answers || reply.RecursionAvailable {
			t.Fatalf("%s: %+v", tt.name, reply)
		}
		if tt.answers > 0 {
			a := reply.Answers[0]
			if a.Header.TTL != 0 || a.Body.(*dnsmessage.AResource).A != [4]byte{192, 168, 50, 1} {
				t.Fatal(a)
			}
		}
	}
	if lanShortcutDNSReply([]byte("malformed"), shortcutTestBinding, true) != nil {
		t.Fatal("accepted malformed DNS")
	}
}

type fakeLANShortcutRunner struct {
	lease   *fakeLANShortcutLease
	err     error
	binding lanSettings
}

func (f *fakeLANShortcutRunner) AcquireLANShortcut(_ context.Context, _ string, b lanSettings) (LANShortcutLease, error) {
	f.binding = b
	if f.err != nil {
		return nil, f.err
	}
	f.lease = &fakeLANShortcutLease{done: make(chan error)}
	return f.lease, nil
}

type fakeLANShortcutLease struct {
	done chan error
	once sync.Once
}

func (f *fakeLANShortcutLease) Done() <-chan error { return f.done }
func (f *fakeLANShortcutLease) Close() error       { f.once.Do(func() { close(f.done) }); return nil }

func TestLANShortcutLifecycleAndIPFallback(t *testing.T) {
	s, c := testLANController(t)
	runner := &fakeLANShortcutRunner{}
	c.shortcutRunner = runner
	configureLAN(t, s, true, testLANPassword)
	if c.response().ShortcutURL != "http://open.surge/" || runner.binding != c.config.lanSettings {
		t.Fatal(c.response())
	}
	first := runner.lease
	configureLAN(t, s, true, "")
	select {
	case <-first.Done():
	default:
		t.Fatal("save did not release old lease")
	}
	if c.response().URL == "" {
		t.Fatal("missing IP URL")
	}
	runner.err = errors.New("port 80 busy")
	configureLAN(t, s, true, "")
	if state := c.response(); state.State != "listening" || state.URL == "" || state.ShortcutURL != "" || state.ShortcutError == "" {
		t.Fatal(state)
	}
	loginLAN(t, c) // A shortcut conflict cannot break existing authentication.
	runner.err = nil
	configureLAN(t, s, true, "")
	last := runner.lease
	c.addresses = func() ([]lanAddress, error) { return nil, nil }
	c.reconcile()
	select {
	case <-last.Done():
	default:
		t.Fatal("address loss leaked lease")
	}
	if state := c.response(); state.ShortcutURL != "" || state.URL != "" {
		t.Fatal(state)
	}
}

func TestLANShortcutSocketsAndShutdown(t *testing.T) {
	var tcpAddresses []string
	listen := func(network, address string) (net.Listener, error) {
		if (len(tcpAddresses) == 0 && address != "192.168.50.1:80") || (len(tcpAddresses) == 1 && address != config.LANShortcutDNS) {
			t.Fatalf("unexpected bind %s", address)
		}
		l, err := net.Listen(network, "127.0.0.1:0")
		if err == nil {
			tcpAddresses = append(tcpAddresses, l.Addr().String())
		}
		return l, err
	}
	var udpAddr string
	packet := func(network, address string) (net.PacketConn, error) {
		if address != config.LANShortcutDNS {
			t.Fatal(address)
		}
		p, err := net.ListenPacket(network, "127.0.0.1:0")
		if err == nil {
			udpAddr = p.LocalAddr().String()
		}
		return p, err
	}
	addresses := func() ([]lanAddress, error) {
		return []lanAddress{{Interface: "en0", IPv4: "192.168.50.1", PrefixLen: 22}}, nil
	}
	service, err := startLANShortcut(context.Background(), shortcutTestBinding, listen, packet, addresses)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	for _, network := range []string{"udp", "tcp"} {
		endpoint := udpAddr
		if network == "tcp" {
			endpoint = tcpAddresses[1]
		}
		resolver := net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, endpoint)
		}}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		ips, err := resolver.LookupIP(ctx, "ip4", "open.surge")
		cancel()
		if err != nil || len(ips) != 1 || ips[0].String() != "192.168.50.1" {
			t.Fatalf("%s: %v %v", network, ips, err)
		}
	}
	client := http.Client{Timeout: time.Second}
	request, _ := http.NewRequest("GET", "http://"+tcpAddresses[0]+"/", nil)
	request.Host = "open.surge"
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("real loopback peer bypassed subnet check")
	}
	service.Close()
	for _, addr := range tcpAddresses {
		l, err := net.Listen("tcp4", addr)
		if err != nil {
			t.Fatalf("TCP not released: %v", err)
		}
		l.Close()
	}
	p, err := net.ListenPacket("udp4", udpAddr)
	if err != nil {
		t.Fatal(err)
	}
	p.Close()
}

func TestLANShortcutPartialBindRollback(t *testing.T) {
	var first net.Listener
	listen := func(network, address string) (net.Listener, error) {
		if strings.HasSuffix(address, ":80") {
			var err error
			first, err = net.Listen(network, "127.0.0.1:0")
			return first, err
		}
		return nil, errors.New("DNS port busy")
	}
	addresses := func() ([]lanAddress, error) {
		return []lanAddress{{Interface: "en0", IPv4: "192.168.50.1", PrefixLen: 22}}, nil
	}
	_, err := startLANShortcut(context.Background(), shortcutTestBinding, listen, net.ListenPacket, addresses)
	if err == nil {
		t.Fatal("accepted unavailable DNS")
	}
	l, err := net.Listen("tcp4", first.Addr().String())
	if err != nil {
		t.Fatal("leaked HTTP listener", err)
	}
	l.Close()
}

func TestLANShortcutHelperLeaseDisconnectAndShutdown(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "client disconnect", true: "helper shutdown"}[shutdown], func(t *testing.T) {
			dir, err := os.MkdirTemp("", "os-lan-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			path := dir + "/helper.sock"
			listener, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			held := &fakeLANShortcutLease{done: make(chan error)}
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				var request HelperRequest
				if json.NewDecoder(conn).Decode(&request) != nil {
					return
				}
				serveLANShortcutLeaseWithStarter(ctx, conn, request.LANShortcut, nil, func(_ context.Context, b lanSettings) (LANShortcutLease, error) { return held, nil })
			}()
			lease, err := (HelperClient{SocketPath: path}).AcquireLANShortcut(context.Background(), "/unused/config.yaml", shortcutTestBinding)
			if err != nil {
				t.Fatal(err)
			}
			if shutdown {
				cancel()
			} else {
				lease.Close()
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("helper retained shortcut after disconnect")
			}
			select {
			case <-held.Done():
			default:
				t.Fatal("helper leaked sockets")
			}
			select {
			case <-lease.Done():
			case <-time.After(time.Second):
				t.Fatal("control service missed helper loss")
			}
			lease.Close()
		})
	}
}

func TestLANShortcutHelperRejectsBeforeLease(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		server, client := net.Pipe()
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer server.Close()
			var validationErr error
			if invalid {
				validationErr = errors.New("untrusted config")
			}
			serveLANShortcutLeaseWithStarter(context.Background(), server, &shortcutTestBinding, validationErr, func(context.Context, lanSettings) (LANShortcutLease, error) {
				if invalid {
					t.Error("started lease despite trust failure")
				}
				// A typed nil returned by the real starter on a bind error is not a live lease.
				var failed *lanShortcutService
				return failed, errors.New("port 80 occupied")
			})
		}()
		var response HelperResponse
		if err := json.NewDecoder(client).Decode(&response); err != nil {
			t.Fatal(err)
		}
		client.Close()
		<-done
		if response.OK || response.Error == "" {
			t.Fatal(response)
		}
	}
}
