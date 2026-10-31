package gateway

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/macosipv6"
	"open-mihomo-gateway/internal/runtime"
)

func ipv6CleanupFixture(t *testing.T) (Manager, *fakeIPv6Host, *[]string) {
	t.Helper()
	cfg := gatewayTestConfig()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	state := runtime.State{
		StartedAt: time.Now(), BootSessionID: "boot-a", IPForwardingBefore: "0",
		PIDDNSMasq: 11, PIDMihomo: 12, PIDIPv6Packet: 13,
		DNSMasqProcessFingerprint: "pid-11-start", MihomoProcessFingerprint: "pid-12-start", IPv6PacketFingerprint: "pid-13-start",
		TUNIPv6Requested: config.TUNIPv6Always, IPv6PacketEffective: true, IPv6GatewayAliasOwned: true, IPv6RAEffective: true,
	}
	if err := runtime.SaveState(paths.StateFile, state); err != nil {
		t.Fatal(err)
	}
	events := []string{}
	host := &fakeIPv6Host{events: &events}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, saveState: runtime.SaveState, removeState: runtime.RemoveState,
		newDHCP:        func(config.Config, runtime.Paths) dhcpService { return &fakeDHCP{events: &events} },
		newMihomo:      func(config.Config, runtime.Paths) mihomoService { return &fakeMihomo{events: &events} },
		newPF:          func(config.Config, runtime.Paths) pfService { return &fakePF{} },
		newSysctl:      func() sysctlService { return &fakeSysctl{} },
		newIPv6Host:    func(config.Config) ipv6HostService { return host },
		newIPv6Packet:  func(config.Config, runtime.Paths) ipv6PacketService { return &fakeIPv6Packet{events: &events} },
		currentBoot:    func() (runtime.BootSession, error) { return runtime.BootSession{ID: "boot-a"}, nil },
		processMatches: fakeProcessMatches,
		stopPrepared:   func(config.Config) error { return nil },
	}}
	return manager, host, &events
}

func TestIPv6UnavailableWithdrawalDoesNotRetainStoppedRuntime(t *testing.T) {
	m, host, events := ipv6CleanupFixture(t)
	host.withdrawErr = macosipv6.ErrWithdrawalUnavailable
	var notices []string
	ctx := WithProgress(t.Context(), func(p Progress) { notices = append(notices, p.Notice) })
	if err := m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(notices, "ipv6_withdrawal_unavailable") {
		t.Fatal("unavailable withdrawal was silently reported as successful", notices)
	}
	assertEventOrder(t, *events, "dhcp-stop", "ipv6-withdraw", "ipv6-gateway-remove", "ipv6-packet-stop", "mihomo-stop")
	if _, exists, err := runtime.LoadState(m.paths.StateFile); err != nil || exists {
		t.Fatal("unavailable downstream link retained stopped runtime", exists, err)
	}
}

func TestIPv6CleanupPreservesUnknownFailuresAndPIDProtection(t *testing.T) {
	for _, source := range []string{"withdrawal inspection", "alias removal", "broker identity"} {
		t.Run(source, func(t *testing.T) {
			m, host, events := ipv6CleanupFixture(t)
			failure := errors.New(source + " failed")
			host.withdrawErr = macosipv6.ErrWithdrawalUnavailable
			switch source {
			case "withdrawal inspection":
				host.withdrawErr = failure
			case "alias removal":
				host.removeErr = failure
			case "broker identity":
				m.deps.processMatches = func(pid int, fingerprint string) (bool, error) {
					if pid == 13 {
						return false, failure
					}
					return fakeProcessMatches(pid, fingerprint)
				}
			}
			if err := m.Stop(t.Context()); !errors.Is(err, failure) {
				t.Fatalf("lost cleanup failure: %v", err)
			}
			if !slices.Contains(*events, "ipv6-gateway-remove") || !slices.Contains(*events, "mihomo-stop") {
				t.Fatal("failure skipped remaining local cleanup", *events)
			}
			if source == "broker identity" && slices.Contains(*events, "ipv6-packet-stop") {
				t.Fatal("signalled a broker without verified process identity", *events)
			}
			if _, exists, err := runtime.LoadState(m.paths.StateFile); err != nil || !exists {
				t.Fatal("lost recovery state for an unresolved cleanup failure", exists, err)
			}
		})
	}
}

func TestIPv6UnavailableWithdrawalDoesNotBlockRollback(t *testing.T) {
	m, host, events := ipv6CleanupFixture(t)
	host.withdrawErr = macosipv6.ErrWithdrawalUnavailable
	state, _, err := runtime.LoadState(m.paths.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("startup failed after IPv6 takeover")
	err = m.rollback(context.Background(), cause, state,
		m.deps.newDHCP(m.cfg, m.paths), m.deps.newMihomo(m.cfg, m.paths),
		m.deps.newPF(m.cfg, m.paths), m.deps.newSysctl(), nil, false)
	if err != cause {
		t.Fatalf("withdrawal warning changed rollback result: %v", err)
	}
	if !slices.Contains(*events, "ipv6-packet-stop") || !slices.Contains(*events, "mihomo-stop") {
		t.Fatal("rollback skipped remaining services", *events)
	}
	if _, exists, err := runtime.LoadState(m.paths.StateFile); err != nil || exists {
		t.Fatal("withdrawal warning retained rollback runtime", exists, err)
	}
}
