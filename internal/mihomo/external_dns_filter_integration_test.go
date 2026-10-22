package mihomo

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/tests/integration/dnsfilterfixture"
)

func TestExternalDNSFilterRealCore(t *testing.T) {
	binary := os.Getenv("OMG_PREPARED_MIHOMO_BINARY")
	if binary == "" {
		t.Skip("set OMG_PREPARED_MIHOMO_BINARY to the patched core")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	control := filepath.Join(t.TempDir(), "decision")
	fixture, err := dnsfilterfixture.Start("127.0.0.1:0", control)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Binary = binary
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "gateway.yaml")
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = filepath.Join(cfg.Runtime.Dir, "source.yaml")
	cfg.DNS.IPv6 = true
	cfg.ExternalDNSFilter = config.ExternalDNSFilterConfig{Enabled: true, Server: fixture.Address(), TimeoutMS: 200, BlockedAddresses: []string{"192.0.2.99"}}
	source := `hosts:
  blocked.example: 192.0.2.7
  fixed.example: 192.0.2.8
  printer.local: 192.0.2.9
  printer.home.arpa: 192.0.2.10
dns:
  enable: true
  listen: 0.0.0.0:7874
  use-hosts: true
  use-system-hosts: false
  nameserver: [rcode://success]
  proxy-server-nameserver: ['udp://127.0.0.1:7874']
  fake-ip-filter: [real.example]
  nameserver-policy: {real.example: rcode://name_error}
rules: ['MATCH,DIRECT']
`
	if err := os.WriteFile(cfg.Mihomo.Profile, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	state, err := Prepare(ctx, cfg, rendered)
	t.Cleanup(func() {
		if err := StopPrepared(cfg); err != nil {
			t.Error(err)
		}
	})
	if err != nil {
		logPreparedFailure(t, cfg)
		t.Fatal(err)
	}
	if fixture.Queries.Load() != 0 {
		t.Fatal("readiness probe reached external filter")
	}
	query := func(network, name string, qtype dnsmessage.Type) dnsmessage.Message {
		t.Helper()
		r, err := dnsfilterfixture.Exchange(network, state.DNSAddr, name, qtype)
		if err != nil {
			logPreparedFailure(t, cfg)
			t.Fatal(err)
		}
		return r
	}
	for _, network := range []string{"udp", "tcp"} {
		for _, qtype := range []dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypeAAAA} {
			r := query(network, "allowed.example", qtype)
			if len(r.Answers) != 1 {
				t.Fatal("allow did not reach Fake-IP", r)
			}
			var ip string
			switch a := r.Answers[0].Body.(type) {
			case *dnsmessage.AResource:
				ip = net.IP(a.A[:]).String()
			case *dnsmessage.AAAAResource:
				ip = net.IP(a.AAAA[:]).String()
			}
			if !strings.HasPrefix(ip, "198.18.") && !strings.HasPrefix(ip, "fdfe:dcba:9876:") {
				t.Fatal("external real address replaced Fake-IP", ip)
			}
			for _, tc := range []struct {
				name  string
				rcode dnsmessage.RCode
			}{{"blocked", dnsmessage.RCodeNameError}, {"refused", dnsmessage.RCodeRefused}, {"fail", dnsmessage.RCodeServerFailure}, {"empty", dnsmessage.RCodeSuccess}, {"timeout", dnsmessage.RCodeServerFailure}, {"truncated", dnsmessage.RCodeRefused}} {
				r := query(network, tc.name+".example", qtype)
				if r.RCode != tc.rcode || len(r.Answers) != 0 {
					t.Fatalf("%s/%s/%v: %+v", network, tc.name, qtype, r)
				}
			}
			r = query(network, "sink.example", qtype)
			if r.RCode != dnsmessage.RCodeSuccess || len(r.Answers) != 1 {
				t.Fatal("missing sink response", r)
			}
			switch a := r.Answers[0].Body.(type) {
			case *dnsmessage.AResource:
				if qtype != dnsmessage.TypeA || a.A != [4]byte{} {
					t.Fatal("incorrect IPv4 sink", r)
				}
			case *dnsmessage.AAAAResource:
				if qtype != dnsmessage.TypeAAAA || a.AAAA != [16]byte{} {
					t.Fatal("incorrect IPv6 sink", r)
				}
			default:
				t.Fatal("unexpected sink record", r)
			}
		}
		r := query(network, "custom.example", dnsmessage.TypeA)
		if len(r.Answers) != 1 || r.Answers[0].Body.(*dnsmessage.AResource).A != [4]byte{192, 0, 2, 99} {
			t.Fatal(r)
		}
		r = query(network, "real.example", dnsmessage.TypeA)
		if r.RCode != dnsmessage.RCodeNameError {
			t.Fatal("real-IP exception lost resolver policy", r)
		}
		r = query(network, "fixed.example", dnsmessage.TypeA)
		if len(r.Answers) != 1 || r.Answers[0].Body.(*dnsmessage.AResource).A != [4]byte{192, 0, 2, 8} {
			t.Fatal("normal hosts result changed", r)
		}
	}
	before := fixture.Queries.Load()
	for _, name := range []string{"printer.local", "printer.home.arpa"} {
		if r := query("udp", name, dnsmessage.TypeA); len(r.Answers) != 1 || fixture.Queries.Load() != before {
			t.Fatal("local hosts boundary", r)
		}
	}
	_ = query("udp", "flip.example", dnsmessage.TypeA)
	if err := os.WriteFile(control, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	if r := query("tcp", "flip.example", dnsmessage.TypeA); r.RCode != dnsmessage.RCodeNameError {
		t.Fatal("cached Fake-IP escaped recheck", r)
	}
	fixture.Close()
	if r := query("udp", "allowed.example", dnsmessage.TypeA); r.RCode != dnsmessage.RCodeServerFailure {
		t.Fatal("filter outage silently bypassed", r)
	}
	cfg.ExternalDNSFilter.Enabled = false
	rendered, err = RenderConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	oldEndpoint := state.DNSAddr
	state, err = Prepare(ctx, cfg, rendered)
	if err != nil {
		t.Fatal(err)
	}
	if state.DNSAddr != oldEndpoint {
		assertDNSPortReleased(t, oldEndpoint)
	}
	if r := query("tcp", "allowed.example", dnsmessage.TypeA); len(r.Answers) != 1 || !strings.HasPrefix(net.IP(r.Answers[0].Body.(*dnsmessage.AResource).A[:]).String(), "198.18.") {
		t.Fatal("disable did not restore Fake-IP", r)
	}
	if err := StopPrepared(cfg); err != nil {
		t.Fatal(err)
	}
	assertDNSPortReleased(t, state.DNSAddr)
}
