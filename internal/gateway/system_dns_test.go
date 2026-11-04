package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

type fakeSystemDNS struct {
	events                *[]string
	path                  string
	backupPath            string
	t                     *testing.T
	enableErr, restoreErr error
	serviceMissing        bool
	preparedServiceID     string
	preparedServers       []string
	restored              []runtime.SystemDNSSnapshot
	restoreErrors         map[string]error
}

func (f *fakeSystemDNS) Prepare(context.Context, string) (runtime.SystemDNSSnapshot, error) {
	*f.events = append(*f.events, "dns-prepare")
	id := f.preparedServiceID
	if id == "" {
		id = "test-service"
	}
	return runtime.SystemDNSSnapshot{ServiceID: id, Interface: "wan0", NetworkService: "Wi-Fi", Servers: slices.Clone(f.preparedServers), Resolvers: []string{"192.168.1.1"}}, nil
}
func (f *fakeSystemDNS) Enable(_ context.Context, snapshot runtime.SystemDNSSnapshot) error {
	state, exists, err := runtime.LoadState(f.path)
	if err != nil || !exists || state.LocalSystemDNS == nil || !state.LocalSystemDNS.Owned || !snapshot.Owned {
		f.t.Fatal("DNS write without durable intent")
	}
	backups, err := runtime.LoadSystemDNSBackups(f.backupPath)
	if err != nil || !backups[snapshot.ServiceID].Owned || !slices.Equal(backups[snapshot.ServiceID].Servers, snapshot.Servers) {
		f.t.Fatal("DNS write without independent original backup", backups, err)
	}
	*f.events = append(*f.events, "dns-enable")
	return f.enableErr
}
func (f *fakeSystemDNS) Restore(_ context.Context, snapshot runtime.SystemDNSSnapshot) (bool, error) {
	*f.events = append(*f.events, "dns-restore")
	f.restored = append(f.restored, snapshot)
	return !f.serviceMissing, errors.Join(f.restoreErr, f.restoreErrors[snapshot.ServiceID])
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
	dns := &fakeSystemDNS{events: &events, path: paths.StateFile, backupPath: paths.SystemDNSBackupFile, t: t}
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

func assertDNSRuntimeCleared(t *testing.T, m Manager) {
	t.Helper()
	if _, exists, err := runtime.LoadState(m.paths.StateFile); err != nil || exists {
		t.Fatal("gateway state still blocks a new start", exists, err)
	}
}

func TestSystemDNSRollbackContinuesAndRetriesBackupWithoutRuntime(t *testing.T) {
	m, dns, engine, events := dnsLifecycleFixture(t)
	dns.preparedServers = []string{"9.9.9.9"}
	dns.enableErr = errors.New("partial DNS write")
	dns.restoreErr = errors.New("restore failed")
	var notices []string
	ctx := WithProgress(t.Context(), func(p Progress) { notices = append(notices, p.Notice) })
	if err := m.Start(ctx); err == nil || !strings.Contains(err.Error(), "partial DNS write") {
		t.Fatal("lost startup failure", err)
	}
	if !engine.stopCalled || indexOfEvent(*events, "dhcp-stop") < 0 {
		t.Fatal("DNS error prevented rollback cleanup", *events)
	}
	assertDNSRuntimeCleared(t, m)
	backups, err := runtime.LoadSystemDNSBackups(m.paths.SystemDNSBackupFile)
	if err != nil || !slices.Equal(backups["test-service"].Servers, dns.preparedServers) {
		t.Fatal("lost original DNS after failed rollback", backups, err)
	}
	if !slices.Contains(notices, "system_dns_restore_failed") {
		t.Fatal("DNS restore warning was hidden", notices)
	}
	dns.restoreErr = nil
	*events = nil
	if err := m.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if indexOfEvent(*events, "dns-restore") < 0 || indexOfEvent(*events, "mihomo-stop") >= 0 {
		t.Fatal("retry without runtime should only restore backups", *events)
	}
	if _, err := os.Stat(m.paths.SystemDNSBackupFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("restored backup was not cleared", err)
	}
}

func TestSystemDNSRestoreFailureDoesNotBlockStopOrNextStart(t *testing.T) {
	for _, reboot := range []bool{false, true} {
		t.Run(fmt.Sprint("reboot=", reboot), func(t *testing.T) {
			m, dns, _, events := dnsLifecycleFixture(t)
			pf := &fakePF{loaded: true}
			forwarding := &fakeSysctl{current: "0"}
			m.deps.newPF = func(config.Config, runtime.Paths) pfService { return pf }
			m.deps.newSysctl = func() sysctlService { return forwarding }
			dns.preparedServers = []string{"9.9.9.9", "149.112.112.112"}
			if err := m.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			// Simulate an upgrade from a release that only saved DNS in state.json.
			if err := os.Remove(m.paths.SystemDNSBackupFile); err != nil {
				t.Fatal(err)
			}
			*events = nil
			dns.restoreErr = errors.New("networksetup unavailable")
			if reboot {
				m.deps.currentBoot = func() (runtime.BootSession, error) { return runtime.BootSession{ID: "next-boot"}, nil }
			}
			if err := m.Stop(t.Context()); err != nil {
				t.Fatal("DNS failure blocked Stop", err)
			}
			assertDNSRuntimeCleared(t, m)
			if indexOfEvent(*events, "dns-restore") < 0 {
				t.Fatal("did not attempt DNS restoration", *events)
			}
			if reboot {
				if indexOfEvent(*events, "mihomo-stop") >= 0 || indexOfEvent(*events, "dhcp-stop") >= 0 || pf.unloadCalled || forwarding.restoreValue != "" {
					t.Fatal("previous boot cleanup changed current boot resources", *events)
				}
			} else if indexOfEvent(*events, "mihomo-stop") < 0 || indexOfEvent(*events, "dhcp-stop") < 0 || !pf.unloadCalled || forwarding.restoreValue != "0" {
				t.Fatal("DNS failure blocked other cleanup", *events)
			}
			// A fresh read still sees our failed-to-restore setting. It must not
			// overwrite the original backup on the next takeover.
			dns.preparedServers = []string{config.LocalSystemDNSServer}
			if err := m.Start(t.Context()); err != nil {
				t.Fatal("pending DNS backup blocked new start", err)
			}
			state, _, _ := runtime.LoadState(m.paths.StateFile)
			if !slices.Equal(state.LocalSystemDNS.Servers, []string{"9.9.9.9", "149.112.112.112"}) {
				t.Fatal("backed up our own DNS setting", state.LocalSystemDNS)
			}
			dns.restoreErr = nil
			if err := m.Stop(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSystemDNSMissingUUIDDoesNotBlockCleanupOrInheritNewService(t *testing.T) {
	for _, reboot := range []bool{false, true} {
		t.Run(fmt.Sprint("reboot=", reboot), func(t *testing.T) {
			m, dns, _, events := dnsLifecycleFixture(t)
			dns.preparedServiceID = "old-service"
			dns.preparedServers = []string{"9.9.9.9"}
			if err := m.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			dns.serviceMissing = true
			if reboot {
				m.deps.currentBoot = func() (runtime.BootSession, error) { return runtime.BootSession{ID: "next-boot"}, nil }
			}
			*events = nil
			if err := m.Stop(t.Context()); err != nil {
				t.Fatal(err)
			}
			assertDNSRuntimeCleared(t, m)
			backups, err := runtime.LoadSystemDNSBackups(m.paths.SystemDNSBackupFile)
			if err != nil || len(backups) != 0 {
				t.Fatal("missing service retained backup", backups, err)
			}
			dns.serviceMissing = false
			dns.preparedServiceID = "new-service"
			dns.preparedServers = []string{"1.1.1.1"}
			if err := m.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			state, _, _ := runtime.LoadState(m.paths.StateFile)
			if state.LocalSystemDNS.ServiceID != "new-service" || !slices.Equal(state.LocalSystemDNS.Servers, []string{"1.1.1.1"}) {
				t.Fatal("replacement inherited old DNS backup", state.LocalSystemDNS)
			}
			if err := m.Stop(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSystemDNSRestartContinuesAfterRestoreFailure(t *testing.T) {
	m, dns, _, events := dnsLifecycleFixture(t)
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	dns.restoreErr = errors.New("temporary DNS restore error")
	*events = nil
	if err := m.RestartMihomo(t.Context()); err != nil {
		t.Fatal(err)
	}
	if indexOfEvent(*events, "mihomo-stop") < 0 || indexOfEvent(*events, "mihomo-start") < 0 || indexOfEvent(*events, "dns-enable") < 0 {
		t.Fatal("DNS restore failure prevented engine replacement", *events)
	}
	dns.restoreErr = nil
	if err := m.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestSystemDNSRestartSkipsMissingService(t *testing.T) {
	m, dns, _, events := dnsLifecycleFixture(t)
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	dns.serviceMissing = true
	*events = nil
	if err := m.RestartMihomo(t.Context()); err != nil {
		t.Fatal(err)
	}
	if indexOfEvent(*events, "mihomo-start") < 0 || indexOfEvent(*events, "dns-enable") >= 0 {
		t.Fatal("restart attempted to reapply missing DNS service", *events)
	}
	if err := m.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestSystemDNSFailedRestartDoesNotReapplyDNS(t *testing.T) {
	m, _, engine, events := dnsLifecycleFixture(t)
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	*events = nil
	engine.startErr = errors.New("restart failed")
	if err := m.RestartMihomo(t.Context()); err == nil {
		t.Fatal("expected restart failure")
	}
	if indexOfEvent(*events, "dns-restore") < 0 || indexOfEvent(*events, "dns-enable") >= 0 {
		t.Fatal("failed restart reapplied DNS", *events)
	}
	if err := m.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestSystemDNSBackupsKeepOnlyFailedServices(t *testing.T) {
	m, dns, _, _ := dnsLifecycleFixture(t)
	if err := runtime.Ensure(m.paths); err != nil {
		t.Fatal(err)
	}
	backups := map[string]runtime.SystemDNSSnapshot{
		"old-service":   {ServiceID: "old-service", Owned: true, Servers: []string{"9.9.9.9"}},
		"other-service": {ServiceID: "other-service", Owned: true},
	}
	if err := runtime.SaveSystemDNSBackups(m.paths.SystemDNSBackupFile, backups); err != nil {
		t.Fatal(err)
	}
	dns.restoreErrors = map[string]error{"old-service": errors.New("service temporarily unreadable")}
	if err := m.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	remaining, err := runtime.LoadSystemDNSBackups(m.paths.SystemDNSBackupFile)
	if err != nil || len(remaining) != 1 || !remaining["old-service"].Owned {
		t.Fatal(remaining, err)
	}
	if len(dns.restored) != 2 {
		t.Fatal("one failure blocked the other service", dns.restored)
	}
}

func TestSystemDNSFailureDoesNotHideOtherCleanupFailure(t *testing.T) {
	m, dns, _, events := dnsLifecycleFixture(t)
	pf := &fakePF{loaded: true, unloadErr: errors.New("pf unload failed")}
	forwarding := &fakeSysctl{current: "0"}
	m.deps.newPF = func(config.Config, runtime.Paths) pfService { return pf }
	m.deps.newSysctl = func() sysctlService { return forwarding }
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	dns.restoreErr = errors.New("DNS restore failed")
	if err := m.Stop(t.Context()); err == nil || !strings.Contains(err.Error(), "pf unload failed") {
		t.Fatal(err)
	}
	if !pf.unloadCalled || forwarding.restoreValue != "0" || indexOfEvent(*events, "mihomo-stop") < 0 {
		t.Fatal("cleanup short circuited", *events)
	}
	if _, exists, err := runtime.LoadState(m.paths.StateFile); err != nil || !exists {
		t.Fatal("lost unresolved PF cleanup state", err)
	}
	pf.unloadErr, dns.restoreErr = nil, nil
	if err := m.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertDNSRuntimeCleared(t, m)
}

func TestMacTUNIPv6RestartUsesAppliedPreference(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		m, _, _, _ := dnsLifecycleFixture(t)
		m.cfg.Transparent.MacTUNIPv6 = enabled
		if err := m.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		state, _, err := runtime.LoadState(m.paths.StateFile)
		if err != nil || state.MacTUNIPv6 != enabled {
			t.Fatal("applied host IPv6 missing", state, err)
		}
		m.cfg.Transparent.MacTUNIPv6 = !enabled // saved preference is not yet applied
		applied := appliedConfigFromState(m.cfg, state)
		if applied.Transparent.MacTUNIPv6 != enabled {
			t.Fatal("restart used unapplied preference")
		}
		want := "disabled"
		if enabled {
			want = "ready"
		}
		if got := macTUNIPv6Status(state, true, "active", "ready"); got != want {
			t.Fatalf("status=%s want=%s", got, want)
		}
		if err := m.RestartMihomo(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := m.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
}
