package dns

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/component/fakeip"
	C "github.com/metacubex/mihomo/constant"
	icontext "github.com/metacubex/mihomo/context"
	D "github.com/miekg/dns"
)

func openSurgeFilterFixture(t *testing.T, fn func(D.ResponseWriter, *D.Msg)) string {
	t.Helper()
	// A free TCP port can still be occupied by an unrelated UDP socket.
	var tcp net.Listener
	var udp net.PacketConn
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		tcp, err = net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		udp, err = net.ListenPacket("udp4", tcp.Addr().String())
		if err == nil {
			break
		}
		_ = tcp.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	servers := []*D.Server{{Listener: tcp, Handler: D.HandlerFunc(fn)}, {PacketConn: udp, Handler: D.HandlerFunc(fn)}}
	for _, server := range servers {
		ready := make(chan struct{})
		server.NotifyStartedFunc = func() { close(ready) }
		go server.ActivateAndServe()
		<-ready
		t.Cleanup(func() { _ = server.Shutdown() })
	}
	return tcp.Addr().String()
}

func openSurgeAnswer(r *D.Msg, ip string) *D.Msg {
	response := new(D.Msg).SetReply(r)
	header := D.RR_Header{Name: r.Question[0].Name, Class: D.ClassINET, Ttl: 600}
	if strings.Contains(ip, ":") {
		header.Rrtype = D.TypeAAAA
		response.Answer = []D.RR{&D.AAAA{Hdr: header, AAAA: net.ParseIP(ip)}}
	} else {
		header.Rrtype = D.TypeA
		response.Answer = []D.RR{&D.A{Hdr: header, A: net.ParseIP(ip)}}
	}
	return response
}

func TestOpenSurgeFilterBeforeFakeIP(t *testing.T) {
	var blocked atomic.Bool
	var lookups atomic.Int32
	address := openSurgeFilterFixture(t, func(w D.ResponseWriter, r *D.Msg) {
		lookups.Add(1)
		ip := "203.0.113.7"
		if r.Question[0].Qtype == D.TypeAAAA {
			ip = "2001:db8::7"
		}
		if blocked.Load() {
			ip = "0.0.0.0"
		}
		_ = w.WriteMsg(openSurgeAnswer(r, ip))
	})
	pool, err := fakeip.New(fakeip.Options{IPNet: netip.MustParsePrefix("198.18.0.0/16"), Size: 1000})
	if err != nil {
		t.Fatal(err)
	}
	pool6, err := fakeip.New(fakeip.Options{IPNet: netip.MustParsePrefix("fdfe:dcba:9876::/64"), Size: 1000})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(nil, NewEnhancer(EnhancerConfig{IPv6: true, EnhancedMode: C.DNSFakeIP, FakeIPPool: pool, FakeIPPool6: pool6, FakeIPSkipper: &fakeip.Skipper{},
		OpenSurgeFilter: OpenSurgeFilterConfig{Enabled: true, Server: address, TimeoutMS: 500}}))
	for _, qtype := range []uint16{D.TypeA, D.TypeAAAA} {
		request := new(D.Msg).SetQuestion("allowed.example.", qtype)
		response, err := service.ServeMsg(t.Context(), request)
		if err != nil || len(response.Answer) != 1 {
			t.Fatalf("allowed: %v %v", response, err)
		}
		if qtype == D.TypeA && !strings.HasPrefix(response.Answer[0].(*D.A).A.String(), "198.18.") {
			t.Fatal(response)
		}
		if qtype == D.TypeAAAA && !strings.HasPrefix(response.Answer[0].(*D.AAAA).AAAA.String(), "fdfe:dcba:9876:") {
			t.Fatal(response)
		}
	}
	// A persisted/in-memory mapping must not bypass a subsequent block decision.
	blocked.Store(true)
	response, err := service.ServeMsg(t.Context(), new(D.Msg).SetQuestion("allowed.example.", D.TypeA))
	if err != nil || response.Answer[0].(*D.A).A.String() != "0.0.0.0" {
		t.Fatalf("stale allow: %v %v", response, err)
	}
	if lookups.Load() != 3 {
		t.Fatalf("checks = %d", lookups.Load())
	}
}

func TestOpenSurgeFilterResponseSemantics(t *testing.T) {
	for _, mode := range []string{"NXDOMAIN", "REFUSED", "SERVFAIL", "NODATA", "A", "AAAA", "custom", "CNAME-sink", "allowed", "tcp", "bad-question", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			var tcpCalls atomic.Int32
			address := openSurgeFilterFixture(t, func(w D.ResponseWriter, r *D.Msg) {
				response := new(D.Msg).SetReply(r)
				switch mode {
				case "NXDOMAIN":
					response.Rcode = D.RcodeNameError
				case "REFUSED":
					response.Rcode = D.RcodeRefused
				case "SERVFAIL":
					response.Rcode = D.RcodeServerFailure
				case "A":
					response = openSurgeAnswer(r, "0.0.0.0")
				case "AAAA":
					response = openSurgeAnswer(r, "::")
				case "custom":
					response = openSurgeAnswer(r, "192.0.2.99")
				case "CNAME-sink":
					response = openSurgeAnswer(r, "0.0.0.0")
					response.Answer = append([]D.RR{&D.CNAME{Hdr: D.RR_Header{Name: r.Question[0].Name, Rrtype: D.TypeCNAME, Class: D.ClassINET}, Target: "sink.example."}}, response.Answer...)
				case "allowed":
					response = openSurgeAnswer(r, "203.0.113.7")
				case "tcp":
					if _, ok := w.RemoteAddr().(*net.UDPAddr); ok {
						response.Truncated = true
					} else {
						tcpCalls.Add(1)
						response.Rcode = D.RcodeRefused
					}
				case "bad-question":
					response.Question[0].Name = "different.example."
				case "timeout":
					return
				}
				_ = w.WriteMsg(response)
			})
			var nextCalls int
			h := withOpenSurgeFilter(OpenSurgeFilterConfig{Server: address, TimeoutMS: 100, BlockedAddresses: []string{"192.0.2.99"}})(func(_ *icontext.DNSContext, r *D.Msg) (*D.Msg, error) {
				nextCalls++
				return openSurgeAnswer(r, "198.18.0.7"), nil
			})
			request := new(D.Msg).SetQuestion("checked.example.", D.TypeA)
			response, err := h(icontext.NewDNSContext(t.Context()), request)
			if err != nil || response == nil || response.Id != request.Id {
				t.Fatalf("response: %v %v", response, err)
			}
			if mode == "allowed" {
				if nextCalls != 1 {
					t.Fatal("normal result did not continue")
				}
			} else if nextCalls != 0 {
				t.Fatal("filter result reached Fake-IP")
			}
			if (mode == "timeout" || mode == "bad-question") && response.Rcode != D.RcodeServerFailure {
				t.Fatal(response)
			}
			if mode == "tcp" && (tcpCalls.Load() != 1 || response.Rcode != D.RcodeRefused) {
				t.Fatal(response, tcpCalls.Load())
			}
		})
	}
}

func TestOpenSurgeFilterBypassAndQueryPrivacy(t *testing.T) {
	var calls atomic.Int32
	address := openSurgeFilterFixture(t, func(w D.ResponseWriter, r *D.Msg) {
		calls.Add(1)
		if len(r.Extra) != 1 || len(r.IsEdns0().Option) != 0 || len(r.Answer) != 0 {
			t.Error("client metadata leaked to filter")
		}
		_ = w.WriteMsg(openSurgeAnswer(r, "203.0.113.7"))
	})
	h := withOpenSurgeFilter(OpenSurgeFilterConfig{Server: address, TimeoutMS: 300, BypassDomains: []string{"private.example"}, ProbeName: "opensurge-ready-exact.invalid"})(func(_ *icontext.DNSContext, r *D.Msg) (*D.Msg, error) { return new(D.Msg).SetReply(r), nil })
	for _, domain := range []string{"printer", "printer.local", "localhost", "printer.lan", "printer.home.arpa", "host.private.example", "private.example", "1.0.0.127.in-addr.arpa", "opensurge-ready-exact.invalid"} {
		_, err := h(icontext.NewDNSContext(t.Context()), new(D.Msg).SetQuestion(domain+".", D.TypeTXT))
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("local name sent to external filter")
	}
	for _, domain := range []string{"notprivate.example", "opensurge-ready-other.invalid"} {
		r := new(D.Msg).SetQuestion(domain+".", D.TypeA)
		r.SetEdns0(1232, true)
		r.IsEdns0().Option = append(r.IsEdns0().Option, &D.EDNS0_SUBNET{Code: D.EDNS0SUBNET, Family: 1, SourceNetmask: 24, Address: net.IPv4(192, 0, 2, 0)})
		_, _ = h(icontext.NewDNSContext(t.Context()), r)
	}
	if calls.Load() != 2 {
		t.Fatal("bypass suffix or probe escaped its exact boundary")
	}
}

func TestOpenSurgeFilterRecursionFailsClosed(t *testing.T) {
	var h handler
	var calls atomic.Int32
	ready := make(chan struct{})
	address := openSurgeFilterFixture(t, func(w D.ResponseWriter, r *D.Msg) {
		<-ready
		calls.Add(1)
		response, _ := h(icontext.NewDNSContext(context.Background()), r)
		_ = w.WriteMsg(response)
	})
	h = withOpenSurgeFilter(OpenSurgeFilterConfig{Server: address, TimeoutMS: 100})(func(_ *icontext.DNSContext, r *D.Msg) (*D.Msg, error) {
		t.Error("recursive filter bypassed")
		return new(D.Msg).SetReply(r), nil
	})
	close(ready)
	start := time.Now()
	response, err := h(icontext.NewDNSContext(t.Context()), new(D.Msg).SetQuestion("loop.example.", D.TypeA))
	if err != nil || response.Rcode != D.RcodeServerFailure || time.Since(start) > time.Second || calls.Load() != 1 {
		t.Fatalf("unbounded recursion: %v %v calls=%d", response, err, calls.Load())
	}
}

func TestOpenSurgeFilterDisabled(t *testing.T) {
	pool, _ := fakeip.New(fakeip.Options{IPNet: netip.MustParsePrefix("198.18.0.0/16"), Size: 1000})
	service := NewService(nil, NewEnhancer(EnhancerConfig{EnhancedMode: C.DNSFakeIP, FakeIPPool: pool, FakeIPSkipper: &fakeip.Skipper{}, OpenSurgeFilter: OpenSurgeFilterConfig{Server: "127.0.0.1:1"}}))
	response, err := service.ServeMsg(t.Context(), new(D.Msg).SetQuestion("disabled.example.", D.TypeA))
	if err != nil || !strings.HasPrefix(response.Answer[0].(*D.A).A.String(), "198.18.") {
		t.Fatal(response, err)
	}
}
