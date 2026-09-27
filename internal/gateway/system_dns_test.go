package gateway

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

type fakeSystemDNS struct {
	events                *[]string
	path                  string
	t                     *testing.T
	enableErr, restoreErr error
	foreign               bool
}

func (f *fakeSystemDNS) Prepare(context.Context, string) (runtime.SystemDNSSnapshot, error) {
	*f.events = append(*f.events, "dns-prepare")
	return runtime.SystemDNSSnapshot{ServiceID: "test-service", Interface: "wan0", NetworkService: "Wi-Fi", Resolvers: []string{"192.168.1.1"}}, nil
}
func (f *fakeSystemDNS) Enable(_ context.Context, snapshot runtime.SystemDNSSnapshot) error {
	state, exists, err := runtime.LoadState(f.path)
	if err != nil || !exists || state.LocalSystemDNS == nil || !state.LocalSystemDNS.Owned || !snapshot.Owned {
		f.t.Fatal("DNS write without durable intent")
	}
	*f.events = append(*f.events, "dns-enable")
	return f.enableErr
}
func (f *fakeSystemDNS) Restore(context.Context, runtime.SystemDNSSnapshot) (bool, error) {
	*f.events = append(*f.events, "dns-restore")
	return !f.foreign, f.restoreErr
}

func dnsLifecycleFixture(t *testing.T) (Manager, *fakeSystemDNS, *fakeMihomo, *[]string) {
	t.Helper()
	cfg := gatewayTestConfig()
	cfg.LocalSystemDNS.Enabled = true
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Gateway.Interface, cfg.Gateway.UpstreamInterface = "lan0", "wan0"
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	events := []string{}
	dns := &fakeSystemDNS{events: &events, path: paths.StateFile, t: t}
	engine := &fakeMihomo{startPID: 12, running: true, events: &events}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, saveState: runtime.SaveState,
		removeState: runtime.RemoveState, ensure: runtime.Ensure,
		newDHCP:            func(config.Config, runtime.Paths) dhcpService { return &fakeDHCP{startPID: 11, events: &events} },
		newMihomo:          func(config.Config, runtime.Paths) mihomoService { return engine },
		newPF:              func(config.Config, runtime.Paths) pfService { return &fakePF{loaded: true, events: &events} },
		newSysctl:          func() sysctlService { return &fakeSysctl{current: "0"} },
		newLocalSystemDNS:  func() localSystemDNSService { return dns },
		processFingerprint: fakeProcessFingerprint, processMatches: fakeProcessMatches,
		currentBoot: func() (runtime.BootSession, error) {
			return runtime.BootSession{ID: "boot", StartedAt: time.Now().Add(-time.Hour)}, nil
		},
		interfaces:      func() ([]net.Interface, error) { return []net.Interface{{Name: "lan0"}, {Name: "wan0"}}, nil },
		interfaceByName: func(name string) (*net.Interface, error) { return &net.Interface{Name: name}, nil },
		interfaceAddrs: func(iface *net.Interface) ([]net.Addr, error) {
			if iface.Name == "lan0" {
				return []net.Addr{&net.IPNet{IP: net.ParseIP(cfg.Gateway.LANIP), Mask: net.CIDRMask(24, 32)}}, nil
			}
			return nil, nil
		}, now: time.Now,
	}}
	return manager, dns, engine, &events
}

func TestSystemDNSLifecycleReadinessRestartAndStop(t *testing.T) {
	m, _, _, events := dnsLifecycleFixture(t)
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if indexOfEvent(*events, "dns-enable") < indexOfEvent(*events, "pf-load") {
		t.Fatal("DNS enabled before readiness", *events)
	}
	state, _, _ := runtime.LoadState(m.paths.StateFile)
	if !state.MacTUNIPv6 || state.LocalSystemDNS.VerifiedAt.IsZero() {
		t.Fatal("missing applied evidence", state)
	}
	*events = nil
	if err := m.RestartMihomo(t.Context()); err != nil {
		t.Fatal(err)
	}
	if indexOfEvent(*events, "dns-restore") > indexOfEvent(*events, "mihomo-stop") || indexOfEvent(*events, "dns-enable") < indexOfEvent(*events, "mihomo-start") {
		t.Fatal("restart ordering", *events)
	}
	*events = nil
	if err := m.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if indexOfEvent(*events, "dns-restore") > indexOfEvent(*events, "dhcp-stop") {
		t.Fatal("stop ordering", *events)
	}
}

func TestSystemDNSFailuresRetainRecoveryAndDoNotStrandResolver(t *testing.T) {
	m, dns, engine, events := dnsLifecycleFixture(t)
	dns.enableErr = errors.New("partial DNS write")
	dns.restoreErr = errors.New("restore failed")
	if err := m.Start(t.Context()); err == nil || !strings.Contains(err.Error(), "retained for recovery") {
		t.Fatal(err)
	}
	if engine.stopCalled {
		t.Fatal("stopped DNS target while restore failed")
	}
	if _, exists, err := runtime.LoadState(m.paths.StateFile); err != nil || !exists {
		t.Fatal("lost recovery snapshot")
	}
	dns.restoreErr = nil
	if err := m.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	dns.enableErr = nil
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	*events = nil
	engine.startErr = errors.New("restart failed")
	if err := m.RestartMihomo(t.Context()); err == nil {
		t.Fatal("expected restart failure")
	}
	if indexOfEvent(*events, "dns-restore") < 0 || indexOfEvent(*events, "dns-enable") >= 0 {
		t.Fatal("failed restart stranded resolver", *events)
	}
	if err := m.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestSystemDNSInterruptedRecoveryAndForeignOwnership(t *testing.T) {
	for _, reboot := range []bool{false, true} {
		m, dns, _, events := dnsLifecycleFixture(t)
		if err := m.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		*events = nil
		if reboot {
			m.deps.currentBoot = func() (runtime.BootSession, error) { return runtime.BootSession{ID: "next-boot"}, nil }
			if err := m.Stop(t.Context()); err != nil {
				t.Fatal(err)
			}
			if indexOfEvent(*events, "dns-restore") < 0 || indexOfEvent(*events, "mihomo-stop") >= 0 {
				t.Fatal("interrupted cleanup", *events)
			}
		} else {
			dns.foreign = true
			if err := m.RestartMihomo(t.Context()); err != nil {
				t.Fatal(err)
			}
			if indexOfEvent(*events, "dns-enable") >= 0 {
				t.Fatal("overwrote foreign DNS", *events)
			}
			if err := m.Stop(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
	}
}
