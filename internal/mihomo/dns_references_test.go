package mihomo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
)

const selfDNSFixture = `dns:
  enable: true
  listen: 0.0.0.0:7874
  nameserver: [192.0.2.53]
  proxy-server-nameserver: [udp://127.0.0.1:7874]
rules: ['MATCH,DIRECT']
`

func renderDNSFixture(t *testing.T, source string) (string, error) {
	t.Helper()
	cfg := config.Default()
	cfg.LocalSystemDNS.Enabled = false
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = filepath.Join(t.TempDir(), "source.yaml")
	if err := os.WriteFile(cfg.Mihomo.Profile, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderConfig(cfg)
	if err == nil {
		again, repeatErr := RenderConfig(cfg)
		if repeatErr != nil || rendered != again {
			t.Fatal("rendering introduced an unstable endpoint or probe identity")
		}
	}
	data, readErr := os.ReadFile(cfg.Mihomo.Profile)
	if readErr != nil || string(data) != source {
		t.Fatal("render changed the source snapshot")
	}
	return rendered, err
}

func TestDNSReferencesClassifyFinalTargets(t *testing.T) {
	for _, tc := range []struct{ name, listen, target, extra, want, wantErr string }{
		{name: "wildcard UDP", listen: "0.0.0.0:7874", target: "udp://127.0.0.1:7874", want: "udp://127.0.0.1:1053"},
		{name: "plain", listen: "127.0.0.1:7874", target: "127.0.0.1:7874", want: "127.0.0.1:1053"},
		{name: "TCP", listen: "0.0.0.0:7874", target: "tcp://127.0.0.1:7874", want: "tcp://127.0.0.1:1053"},
		{name: "IPv6 UDP", listen: "[::]:7874", target: "udp://[::1]:7874", want: "udp://127.0.0.1:1053"},
		{name: "IPv6 TCP", listen: "[::1]:7874", target: "tcp://[::1]:7874", want: "tcp://127.0.0.1:1053"},
		{name: "different loopback", listen: "127.0.0.1:7874", target: "udp://127.0.0.2:7874", want: "udp://127.0.0.2:7874"},
		{name: "different port", listen: "0.0.0.0:7874", target: "tcp://127.0.0.1:5353", want: "tcp://127.0.0.1:5353"},
		{name: "external", listen: "0.0.0.0:7874", target: "https://dns.example:7874/dns-query#DIRECT", want: "https://dns.example:7874/dns-query#DIRECT"},
		{name: "external literal", listen: "0.0.0.0:7874", target: "192.0.2.53:7874", want: "192.0.2.53:7874"},
		{name: "protocol parameters", listen: "0.0.0.0:7874", target: "udp://127.0.0.1:7874#Proxy", wantErr: "unsupported protocol"},
		{name: "TLS", listen: "0.0.0.0:7874", target: "tls://127.0.0.1:7874", wantErr: "unsupported protocol"},
		{name: "hostname alias", listen: "0.0.0.0:7874", target: "localhost:7874", wantErr: "ambiguous"},
		{name: "dual stack ambiguity", listen: "[::]:7874", target: "127.0.0.1:7874", wantErr: "ambiguous"},
		{name: "managed port conflict", listen: "0.0.0.0:7874", target: "127.0.0.1:1053", wantErr: "conflicts"},
		{name: "respect rules", listen: "0.0.0.0:7874", target: "127.0.0.1:7874", extra: "  respect-rules: true\n", wantErr: "bootstrap"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := fmt.Sprintf("dns:\n  enable: true\n  listen: %q\n  nameserver: [192.0.2.53]\n  proxy-server-nameserver: [%q]\n%srules: ['MATCH,DIRECT']\n", tc.listen, tc.target, tc.extra)
			final, err := renderDNSFixture(t, source)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("got %v, want %s", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			refs, err := DNSReferenceBindings(final)
			if err != nil {
				t.Fatal(err)
			}
			ref := refs[len(refs)-1]
			if ref.Target != tc.target || ref.Effective != tc.want || ref.Source != "subscription" || ref.SourceDigest != ProfileOverlayDigest([]byte(source)) {
				t.Fatalf("wrong diagnostic: %+v", ref)
			}
		})
	}
}

func TestDNSReferencesRejectRecursiveFieldsAndPreserveDeclarations(t *testing.T) {
	for _, field := range []string{"nameserver", "default-nameserver", "fallback", "direct-nameserver", "nameserver-policy"} {
		t.Run(field, func(t *testing.T) {
			value := "[127.0.0.1:7874]"
			if field == "nameserver-policy" {
				value = "{'+.test': 127.0.0.1:7874}"
			}
			source := fmt.Sprintf("dns:\n  enable: true\n  listen: 0.0.0.0:7874\n  %s: %s\nrules: ['MATCH,DIRECT']\n", field, value)
			if _, err := renderDNSFixture(t, source); err == nil || !strings.Contains(err.Error(), "resolver dependency") {
				t.Fatalf("accepted self dependency: %v", err)
			}
		})
	}
	for _, enabled := range []bool{false, true} {
		source := selfDNSFixture + `x-opensurge-dns-services:
  - field: 'dns.proxy-server-nameserver[0]'
    target: udp://127.0.0.1:7874
    service: independent
`
		doc := DefaultProfileOverlayDocument()
		doc.Enabled = enabled
		composition, err := ComposeProfileOverlay([]byte(source), doc)
		if err != nil {
			t.Fatal(err)
		}
		final, err := renderDNSFixture(t, composition.ProfileYAML)
		if err != nil {
			t.Fatal(err)
		}
		refs, _ := DNSReferenceBindings(final)
		ref := refs[len(refs)-1]
		if ref.Service != "independent" || ref.Effective != "udp://127.0.0.1:7874" {
			t.Fatalf("declaration lost: %+v", ref)
		}
	}
}

func TestDNSReferenceMetadataSurvivesRepeatedCompositionAndTargetChanges(t *testing.T) {
	doc := DefaultProfileOverlayDocument()
	doc.Enabled = true
	first, err := ComposeProfileOverlay([]byte(selfDNSFixture), doc)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ComposeProfileOverlay([]byte(first.ProfileYAML), doc)
	if err != nil {
		t.Fatal(err)
	}
	if first.ProfileYAML != second.ProfileYAML {
		t.Fatal("metadata drifted on repeated composition")
	}
	for _, source := range []string{selfDNSFixture, first.ProfileYAML, second.ProfileYAML} {
		final, err := renderDNSFixture(t, source)
		if err != nil {
			t.Fatal(err)
		}
		refs, _ := DNSReferenceBindings(final)
		ref := refs[len(refs)-1]
		if ref.Service != "self" || ref.SourceDigest != ProfileOverlayDigest([]byte(selfDNSFixture)) {
			t.Fatalf("source provenance lost: %+v", ref)
		}
	}
	doc.DNS.Merge = map[string]any{"proxy-server-nameserver": []any{"tcp://127.0.0.1:5353#DIRECT"}}
	changed, err := ComposeProfileOverlay([]byte(first.ProfileYAML), doc)
	if err != nil {
		t.Fatal(err)
	}
	final, err := renderDNSFixture(t, changed.ProfileYAML)
	if err != nil {
		t.Fatal(err)
	}
	refs, _ := DNSReferenceBindings(final)
	ref := refs[len(refs)-1]
	if ref.Source != "global-overlay" || ref.Service != "independent" || ref.Target != ref.Effective {
		t.Fatalf("stale self classification: %+v", ref)
	}
	if ref.Overrides == nil || ref.Overrides.Target != "udp://127.0.0.1:7874" || ref.Overrides.Source != "subscription" {
		t.Fatalf("lost override relationship: %+v", ref)
	}
}

func TestDNSReferencePolicyAliasesAndPreparedBindings(t *testing.T) {
	source := strings.Replace(selfDNSFixture, "  proxy-server-nameserver: [udp://127.0.0.1:7874]", "  proxy-server-nameserver: &node [udp://127.0.0.1:7874]\n  proxy-server-nameserver-policy: {'+.node.test': *node}", 1)
	final, err := renderDNSFixture(t, source)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := renderPreparedConfig(final, "127.0.0.1:19090", "secret", "127.0.0.1:19053", "opensurge-ready-fixture.invalid")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prepared, ":1053") || strings.Contains(prepared, ":7874") || strings.Contains(prepared, dnsBindingsKey) {
		t.Fatalf("stale runtime endpoints: %s", prepared)
	}
	formalProbe, err := finalDNSProbeName(final)
	if err != nil || formalProbe == "" || strings.Contains(prepared, formalProbe) {
		t.Fatalf("prepared config retained the formal readiness identity: %v", err)
	}
	var root map[string]any
	if err := yaml.Unmarshal([]byte(prepared), &root); err != nil {
		t.Fatal(err)
	}
	dns := root["dns"].(map[string]any)
	if dns["listen"] != "127.0.0.1:19053" || !reflect.DeepEqual(dns["proxy-server-nameserver"], []any{"udp://127.0.0.1:19053"}) {
		t.Fatalf("prepared DNS not bound: %#v", dns)
	}
	if _, err := RenderPreparedConfig(final, "127.0.0.1:19090", "secret"); err == nil {
		t.Fatal("silently disabled a required listener")
	}
	stale := strings.Replace(final, "udp://127.0.0.1:1053", "udp://127.0.0.1:5555", 1)
	if _, err := renderPreparedConfig(stale, "127.0.0.1:19090", "secret", "127.0.0.1:19053", "opensurge-ready-fixture.invalid"); err == nil {
		t.Fatal("accepted stale binding metadata")
	}
}

func TestFormalDNSReadinessCannotBorrowAnUnownedListener(t *testing.T) {
	manager := Manager{}
	// This test process does not own the formal DNS port, regardless of whether
	// another installed client does. No DNS query is sent without ownership.
	if err := manager.waitForDNS(t.Context(), os.Getpid(), "opensurge-ready-test.invalid", 20*time.Millisecond); err == nil {
		t.Fatal("unowned DNS listener passed formal readiness")
	}
}

func TestPreparedDNSLifecycleReusesAndCleansOwnedResources(t *testing.T) {
	final, err := renderDNSFixture(t, selfDNSFixture)
	if err != nil {
		t.Fatal(err)
	}
	cfg, fake, deps := preparedFixture(t)
	reservations, probes, releases := 0, 0, 0
	deps.reserveDNS = func() (string, func(), error) {
		reservations++
		return "127.0.0.1:19053", func() { releases++ }, nil
	}
	deps.dnsReady = func(_ context.Context, state PreparedState) error {
		probes++
		if state.DNSAddr != "127.0.0.1:19053" || state.DNSProbeName == "" {
			t.Fatal("missing DNS ownership")
		}
		return nil
	}
	first, err := prepareLocked(context.Background(), cfg, final, deps)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareLocked(context.Background(), cfg, final, deps)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || reservations != 1 || probes != 2 || fake.starts != 1 || releases == 0 {
		t.Fatal("private DNS port caused a needless restart or skipped probing")
	}
	deps.dnsReady = func(context.Context, PreparedState) error { return errors.New("UDP listener lost") }
	if _, err := prepareLocked(context.Background(), cfg, final, deps); err == nil || fake.starts != 1 || fake.stops != 0 {
		t.Fatal("live DNS failure spawned a replacement owner")
	}
	if err := stopPreparedLocked(cfg, deps); err != nil {
		t.Fatal(err)
	}
	if _, exists, _ := LoadPrepared(cfg); exists || fake.alive {
		t.Fatal("stop retained prepared ownership")
	}
}

func TestPreparedDNSFailureIsBoundedAndRollbackRetainsOwnershipOnFailure(t *testing.T) {
	final, err := renderDNSFixture(t, selfDNSFixture)
	if err != nil {
		t.Fatal(err)
	}
	cfg, fake, deps := preparedFixture(t)
	attempts := 0
	deps.reserveDNS = func() (string, func(), error) { attempts++; return "", nil, errors.New("TCP/UDP conflict") }
	if _, err := prepareLocked(context.Background(), cfg, final, deps); err == nil || attempts != 3 || fake.starts != 0 {
		t.Fatal("allocation retry was not bounded")
	}
	deps.reserveDNS = reservePreparedDNS
	deps.dnsReady = func(context.Context, PreparedState) error { return errors.New("wrong listener owner") }
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := prepareLocked(ctx, cfg, final, deps); err == nil {
		t.Fatal("controller alone passed readiness")
	}
	if fake.alive {
		t.Fatal("DNS failure left a running core")
	}
	if _, exists, _ := LoadPrepared(cfg); exists {
		t.Fatal("DNS failure retained state after successful cleanup")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	fake.stopErr = errors.New("cannot stop owned process")
	if _, err := prepareLocked(ctx, cfg, final, deps); err == nil {
		t.Fatal("ignored rollback failure")
	}
	if _, exists, _ := LoadPrepared(cfg); !exists || !fake.alive {
		t.Fatal("rollback failure lost ownership evidence")
	}
}

func TestPreparedDNSReservationHoldsBothTransports(t *testing.T) {
	address, release, err := reservePreparedDNS()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if listener, err := net.Listen("tcp4", address); err == nil {
		listener.Close()
		t.Fatal("TCP reservation absent")
	}
	if listener, err := net.ListenPacket("udp4", address); err == nil {
		listener.Close()
		t.Fatal("UDP reservation absent")
	}
	release()
	for _, network := range []string{"tcp4", "udp4"} {
		if network == "tcp4" {
			listener, err := net.Listen(network, address)
			if err != nil {
				t.Fatal(err)
			}
			listener.Close()
		} else {
			listener, err := net.ListenPacket(network, address)
			if err != nil {
				t.Fatal(err)
			}
			listener.Close()
		}
	}
}
