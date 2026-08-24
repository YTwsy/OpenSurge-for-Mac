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
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

func TestWarmManagedTailscaleOnlyWhenEnabled(t *testing.T) {
	calls := 0
	deps := gatewayDeps{warmTailscale: func(context.Context, config.Config) mihomo.ProxyDelayResult {
		calls++
		return mihomo.ProxyDelayResult{Status: "reachable", DelayMS: 12}
	}}
	manager := Manager{cfg: config.Default()}
	manager.warmManagedTailscale(context.Background(), deps)
	if calls != 0 {
		t.Fatalf("disabled Tailscale warm-up calls = %d", calls)
	}
	manager.cfg.Tailscale.Enabled = true
	manager.warmManagedTailscale(context.Background(), deps)
	if calls != 1 {
		t.Fatalf("enabled Tailscale warm-up calls = %d", calls)
	}
}

func TestStartRollsBackWhenMihomoStartFails(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "lan0"
	cfg.Gateway.UpstreamInterface = "wan0"
	cfg.Gateway.LANIP = "192.168.50.1"
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)

	dhcpManager := &fakeDHCP{startPID: 111}
	mihomoManager := &fakeMihomo{startErr: errors.New("mihomo start failed")}
	pfManager := &fakePF{enabled: false}
	sysctlManager := &fakeSysctl{current: "0"}

	manager := Manager{
		cfg:   cfg,
		paths: paths,
		deps: gatewayDeps{
			geteuid:     func() int { return 0 },
			loadState:   runtime.LoadState,
			saveState:   runtime.SaveState,
			removeState: runtime.RemoveState,
			ensure:      runtime.Ensure,
			newDHCP: func(config.Config, runtime.Paths) dhcpService {
				return dhcpManager
			},
			newMihomo: func(config.Config, runtime.Paths) mihomoService {
				return mihomoManager
			},
			newPF: func(config.Config, runtime.Paths) pfService {
				return pfManager
			},
			newSysctl: func() sysctlService {
				return sysctlManager
			},
			interfaces: func() ([]net.Interface, error) {
				return []net.Interface{{Name: cfg.Gateway.Interface}}, nil
			},
			interfaceByName: func(name string) (*net.Interface, error) {
				return &net.Interface{Name: name}, nil
			},
			interfaceAddrs: func(*net.Interface) ([]net.Addr, error) {
				return []net.Addr{&net.IPNet{
					IP:   net.ParseIP(cfg.Gateway.LANIP),
					Mask: net.CIDRMask(24, 32),
				}}, nil
			},
			now: func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		},
	}

	err := manager.Start(context.Background())
	if err == nil {
		t.Fatalf("Start() succeeded")
	}
	if !strings.Contains(err.Error(), "mihomo start failed") {
		t.Fatalf("Start() error = %q", err)
	}
	if !sysctlManager.enableCalled {
		t.Fatalf("sysctl Enable() was not called")
	}
	if sysctlManager.restoreValue != "0" {
		t.Fatalf("sysctl Restore() = %q, want 0", sysctlManager.restoreValue)
	}
	if dhcpManager.startCalled {
		t.Fatalf("dnsmasq Start() was called after mihomo failure")
	}
	if !dhcpManager.stopCalled {
		t.Fatalf("dnsmasq Stop() was not called during rollback")
	}
	if !mihomoManager.stopCalled {
		t.Fatalf("mihomo Stop() was not called during rollback")
	}
	if pfManager.loadCalled {
		t.Fatalf("pf Load() was called before mihomo succeeded")
	}
	if pfManager.unloadCalled {
		t.Fatalf("pf Unload() was called even though anchor was not loaded")
	}
	if _, exists, err := runtime.LoadState(paths.StateFile); err != nil {
		t.Fatalf("LoadState() error = %v", err)
	} else if exists {
		t.Fatalf("runtime state still exists after rollback")
	}
}

func TestPreflightRejectsSameGatewayAndUpstreamInterface(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "en0"
	cfg.Gateway.UpstreamInterface = " en0 "
	manager := Manager{cfg: cfg, paths: runtime.NewPaths(cfg), deps: defaultGatewayDeps()}

	err := manager.preflight(&fakeDHCP{}, &fakeMihomo{}, &fakePF{}, &fakeSysctl{}, manager.deps)
	if err == nil {
		t.Fatalf("preflight() succeeded")
	}
	if !strings.Contains(err.Error(), "must differ") {
		t.Fatalf("preflight() error = %q", err)
	}
}

func TestPreflightAcceptsSameInterfaceInSameLANMode(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Mode = config.GatewayModeSameLAN
	cfg.Gateway.Interface = "en0"
	cfg.Gateway.UpstreamInterface = " en0 "
	cfg.Gateway.LANIP = "192.168.1.20"
	cfg.DHCP.Enabled = false
	cfg.Transparent.Mode = config.TransparentModeTUN
	manager := Manager{
		cfg:   cfg,
		paths: runtime.NewPaths(cfg),
		deps: gatewayDeps{
			interfaceByName: func(name string) (*net.Interface, error) {
				return &net.Interface{Name: strings.TrimSpace(name)}, nil
			},
			interfaces: func() ([]net.Interface, error) {
				return []net.Interface{{Name: "en0"}}, nil
			},
			interfaceAddrs: func(iface *net.Interface) ([]net.Addr, error) {
				return []net.Addr{&net.IPNet{
					IP:   net.ParseIP(cfg.Gateway.LANIP),
					Mask: net.CIDRMask(24, 32),
				}}, nil
			},
		},
	}

	err := manager.preflight(&fakeDHCP{}, &fakeMihomo{}, &fakePF{}, &fakeSysctl{}, manager.deps)
	if err != nil {
		t.Fatalf("preflight() error = %v", err)
	}
}

func TestPreflightAcceptsSameInterfaceInSameWiFiDHCPMode(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Mode = config.GatewayModeSameWiFiDHCP
	cfg.Gateway.Interface = "en0"
	cfg.Gateway.UpstreamInterface = " en0 "
	cfg.Gateway.LANIP = "192.168.1.20"
	cfg.DHCP.Enabled = true
	cfg.DHCP.RangeStart = "192.168.1.120"
	cfg.DHCP.RangeEnd = "192.168.1.199"
	cfg.Transparent.Mode = config.TransparentModeTUN
	manager := Manager{
		cfg:   cfg,
		paths: runtime.NewPaths(cfg),
		deps: gatewayDeps{
			interfaceByName: func(name string) (*net.Interface, error) {
				return &net.Interface{Name: strings.TrimSpace(name)}, nil
			},
			interfaces: func() ([]net.Interface, error) {
				return []net.Interface{{Name: "en0"}}, nil
			},
			interfaceAddrs: func(iface *net.Interface) ([]net.Addr, error) {
				return []net.Addr{&net.IPNet{
					IP:   net.ParseIP(cfg.Gateway.LANIP),
					Mask: net.CIDRMask(24, 32),
				}}, nil
			},
		},
	}

	err := manager.preflight(&fakeDHCP{}, &fakeMihomo{}, &fakePF{}, &fakeSysctl{}, manager.deps)
	if err != nil {
		t.Fatalf("preflight() error = %v", err)
	}
}

func TestPreflightRejectsDifferentInterfacesInSameLANMode(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Mode = config.GatewayModeSameLAN
	cfg.Gateway.Interface = "en0"
	cfg.Gateway.UpstreamInterface = "en7"
	cfg.DHCP.Enabled = false
	cfg.Transparent.Mode = config.TransparentModeTUN
	manager := Manager{cfg: cfg, paths: runtime.NewPaths(cfg), deps: defaultGatewayDeps()}

	err := manager.preflight(&fakeDHCP{}, &fakeMihomo{}, &fakePF{}, &fakeSysctl{}, manager.deps)
	if err == nil {
		t.Fatalf("preflight() succeeded")
	}
	if !strings.Contains(err.Error(), "same_lan requires gateway and upstream interfaces to match") {
		t.Fatalf("preflight() error = %q", err)
	}
}

func TestPreflightRejectsLANIPOnAnotherInterface(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "bridge102"
	cfg.Gateway.UpstreamInterface = "en0"
	cfg.Gateway.LANIP = "192.168.50.1"
	manager := Manager{
		cfg:   cfg,
		paths: runtime.NewPaths(cfg),
		deps: gatewayDeps{
			interfaceByName: func(name string) (*net.Interface, error) {
				return &net.Interface{Name: name}, nil
			},
			interfaces: func() ([]net.Interface, error) {
				return []net.Interface{
					{Name: "bridge102"},
					{Name: "en7"},
				}, nil
			},
			interfaceAddrs: func(iface *net.Interface) ([]net.Addr, error) {
				switch iface.Name {
				case "bridge102", "en7":
					return []net.Addr{&net.IPNet{
						IP:   net.ParseIP(cfg.Gateway.LANIP),
						Mask: net.CIDRMask(24, 32),
					}}, nil
				default:
					return nil, nil
				}
			},
		},
	}

	err := manager.preflight(&fakeDHCP{}, &fakeMihomo{}, &fakePF{}, &fakeSysctl{}, manager.deps)
	if err == nil {
		t.Fatalf("preflight() succeeded")
	}
	if !strings.Contains(err.Error(), "also configured on interface en7") {
		t.Fatalf("preflight() error = %q", err)
	}
}

func TestCheckReservationConflictsRejectsObservedDifferentMACInSameWiFiDHCP(t *testing.T) {
	bundle, err := device.CompilePolicyBundle(device.PolicySet{
		Profiles: []device.Profile{{ID: "home", DefaultPolicies: []string{"DIRECT"}}},
		Devices:  []device.ManagedDevice{{ID: "phone", MAC: "aa:bb:cc:dd:ee:01", IPv4: "192.168.1.101", Profile: "home"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Gateway.Mode = config.GatewayModeSameWiFiDHCP
	cfg.DevicePolicy.Bundle = &bundle
	manager := Manager{cfg: cfg, deps: gatewayDeps{
		probeReservationIP: func(ip, expectedMAC string) error {
			if ip != "192.168.1.101" || expectedMAC != "aa:bb:cc:dd:ee:01" {
				t.Fatalf("probe args = %q/%q", ip, expectedMAC)
			}
			return errors.New("reserved IPv4 already present")
		},
	}}
	if err := manager.checkReservationConflicts(manager.deps); err == nil || !strings.Contains(err.Error(), "already present") {
		t.Fatalf("checkReservationConflicts() error = %v", err)
	}
}

func TestStartValidatesMihomoBeforeEnablingForwarding(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "lan0"
	cfg.Gateway.UpstreamInterface = "wan0"
	cfg.Gateway.LANIP = "192.168.50.1"
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	mihomoManager := &fakeMihomo{validateErr: errors.New("duplicate group name")}
	sysctlManager := &fakeSysctl{current: "0"}
	manager := Manager{
		cfg:   cfg,
		paths: runtime.NewPaths(cfg),
		deps: gatewayDeps{
			geteuid:     func() int { return 0 },
			loadState:   runtime.LoadState,
			saveState:   runtime.SaveState,
			removeState: runtime.RemoveState,
			ensure:      runtime.Ensure,
			newDHCP:     func(config.Config, runtime.Paths) dhcpService { return &fakeDHCP{} },
			newMihomo:   func(config.Config, runtime.Paths) mihomoService { return mihomoManager },
			newPF:       func(config.Config, runtime.Paths) pfService { return &fakePF{} },
			newSysctl:   func() sysctlService { return sysctlManager },
			interfaces:  func() ([]net.Interface, error) { return []net.Interface{{Name: "lan0"}, {Name: "wan0"}}, nil },
			interfaceByName: func(name string) (*net.Interface, error) {
				return &net.Interface{Name: name}, nil
			},
			interfaceAddrs: func(iface *net.Interface) ([]net.Addr, error) {
				if iface.Name != "lan0" {
					return nil, nil
				}
				return []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.50.1"), Mask: net.CIDRMask(24, 32)}}, nil
			},
		},
	}
	if err := manager.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "duplicate group") {
		t.Fatalf("Start() error = %v", err)
	}
	if sysctlManager.enableCalled {
		t.Fatal("Start() enabled forwarding before mihomo validation")
	}
	if _, exists, err := runtime.LoadState(manager.paths.StateFile); err != nil || exists {
		t.Fatalf("runtime state after validation failure = exists=%v err=%v", exists, err)
	}
}

func TestStartAndStopCoordinateLocalSystemProxyAroundGatewayServices(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "lan0"
	cfg.Gateway.UpstreamInterface = "wan0"
	cfg.Gateway.LANIP = "192.168.50.1"
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.LocalSystemProxy.Enabled = true
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	events := []string{}
	dhcpManager := &fakeDHCP{startPID: 11, events: &events}
	mihomoManager := &fakeMihomo{startPID: 12, events: &events}
	pfManager := &fakePF{loaded: true, events: &events}
	systemProxyManager := &fakeLocalSystemProxy{
		snapshot: runtime.SystemProxySnapshot{NetworkService: "USB 10/100/1000 LAN", Interface: "wan0"},
		events:   &events,
	}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, saveState: runtime.SaveState,
		removeState: runtime.RemoveState, ensure: runtime.Ensure,
		newDHCP:             func(config.Config, runtime.Paths) dhcpService { return dhcpManager },
		newMihomo:           func(config.Config, runtime.Paths) mihomoService { return mihomoManager },
		newPF:               func(config.Config, runtime.Paths) pfService { return pfManager },
		newSysctl:           func() sysctlService { return &fakeSysctl{current: "0"} },
		newLocalSystemProxy: func() localSystemProxyService { return systemProxyManager },
		processFingerprint:  fakeProcessFingerprint,
		processMatches:      fakeProcessMatches,
		interfaces: func() ([]net.Interface, error) {
			return []net.Interface{{Name: "lan0"}, {Name: "wan0"}}, nil
		},
		interfaceByName: func(name string) (*net.Interface, error) { return &net.Interface{Name: name}, nil },
		interfaceAddrs: func(iface *net.Interface) ([]net.Addr, error) {
			if iface.Name == "lan0" {
				return []net.Addr{&net.IPNet{IP: net.ParseIP(cfg.Gateway.LANIP), Mask: net.CIDRMask(24, 32)}}, nil
			}
			return nil, nil
		},
		now: time.Now,
	}}

	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if systemProxyManager.prepareInterface != "wan0" || systemProxyManager.preparePort != cfg.Mihomo.MixedPort {
		t.Fatalf("Prepare() interface=%q port=%d", systemProxyManager.prepareInterface, systemProxyManager.preparePort)
	}
	if indexOfEvent(events, "system-proxy-prepare") > indexOfEvent(events, "mihomo-write") {
		t.Fatalf("system proxy conflict check ran after runtime artifacts were written: %v", events)
	}
	state, exists, err := runtime.LoadState(paths.StateFile)
	if err != nil || !exists || state.LocalSystemProxy == nil || state.LocalSystemProxy.NetworkService != "USB 10/100/1000 LAN" {
		t.Fatalf("runtime state=%#v exists=%v err=%v", state, exists, err)
	}
	if indexOfEvent(events, "system-proxy-enable") < indexOfEvent(events, "pf-load") {
		t.Fatalf("system proxy enabled before gateway readiness: %v", events)
	}

	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	restoreIndex := indexOfEvent(events, "system-proxy-restore")
	if restoreIndex == -1 || restoreIndex > indexOfEvent(events, "mihomo-stop") || restoreIndex > indexOfEvent(events, "dhcp-stop") {
		t.Fatalf("system proxy was not restored before gateway services stopped: %v", events)
	}

	events = nil
	systemProxyManager.enableErr = errors.New("proxy endpoint rejected")
	err = manager.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "proxy endpoint rejected") {
		t.Fatalf("Start() proxy enable error=%v", err)
	}
	if systemProxyManager.restoreCalls != 2 {
		t.Fatalf("system proxy restore calls=%d, want stop plus failed-start rollback", systemProxyManager.restoreCalls)
	}
	if _, exists, loadErr := runtime.LoadState(paths.StateFile); loadErr != nil || exists {
		t.Fatalf("runtime state after proxy rollback exists=%v err=%v", exists, loadErr)
	}
}

func TestReloadValidationFailureLeavesRunningGatewayUntouched(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "lan0"
	cfg.Gateway.UpstreamInterface = "wan0"
	cfg.Gateway.LANIP = "192.168.50.1"
	cfg.Runtime.Dir = filepath.Join(t.TempDir(), "runtime")
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	mihomoManager := &fakeMihomo{validateErr: errors.New("candidate rejected"), running: true}
	dhcpManager := &fakeDHCP{running: true}
	manager := Manager{
		cfg: cfg, paths: paths,
		deps: gatewayDeps{
			geteuid: func() int { return 0 }, loadState: runtime.LoadState, saveState: runtime.SaveState,
			removeState: runtime.RemoveState, ensure: runtime.Ensure,
			newDHCP:         func(config.Config, runtime.Paths) dhcpService { return dhcpManager },
			newMihomo:       func(config.Config, runtime.Paths) mihomoService { return mihomoManager },
			newPF:           func(config.Config, runtime.Paths) pfService { return &fakePF{} },
			newSysctl:       func() sysctlService { return &fakeSysctl{} },
			interfaces:      func() ([]net.Interface, error) { return []net.Interface{{Name: "lan0"}, {Name: "wan0"}}, nil },
			interfaceByName: func(name string) (*net.Interface, error) { return &net.Interface{Name: name}, nil },
			interfaceAddrs: func(iface *net.Interface) ([]net.Addr, error) {
				if iface.Name == "lan0" {
					return []net.Addr{&net.IPNet{IP: net.ParseIP(cfg.Gateway.LANIP), Mask: net.CIDRMask(24, 32)}}, nil
				}
				return nil, nil
			},
		},
	}
	err := manager.Reload(context.Background())
	if err == nil || !strings.Contains(err.Error(), "candidate rejected") {
		t.Fatalf("Reload() error=%v", err)
	}
	if dhcpManager.stopCalled || mihomoManager.stopCalled {
		t.Fatal("reload stopped live services after candidate validation failed")
	}
	if _, exists, err := runtime.LoadState(paths.StateFile); err != nil || !exists {
		t.Fatalf("runtime state exists=%v err=%v", exists, err)
	}
}

func TestReloadStopsBeforeRestartAndWritesFreshState(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "lan0"
	cfg.Gateway.UpstreamInterface = "wan0"
	cfg.Gateway.LANIP = "192.168.50.1"
	cfg.Runtime.Dir = filepath.Join(t.TempDir(), "runtime")
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	profile := filepath.Join(filepath.Dir(cfg.Runtime.Dir), "imported.yaml")
	profileData := []byte("proxies: []\nproxy-groups: []\nrules: []\n")
	if err := os.WriteFile(profile, profileData, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = profile
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{PIDDNSMasq: 11, PIDMihomo: 12, IPForwardingBefore: "0", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	events := []string{}
	dhcpManager := &fakeDHCP{running: true, startPID: 21, events: &events}
	mihomoManager := &fakeMihomo{running: true, startPID: 22, events: &events}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, saveState: runtime.SaveState,
		removeState: runtime.RemoveState, ensure: runtime.Ensure,
		newDHCP:            func(config.Config, runtime.Paths) dhcpService { return dhcpManager },
		newMihomo:          func(config.Config, runtime.Paths) mihomoService { return mihomoManager },
		newPF:              func(config.Config, runtime.Paths) pfService { return &fakePF{loaded: true} },
		newSysctl:          func() sysctlService { return &fakeSysctl{current: "0"} },
		processFingerprint: fakeProcessFingerprint,
		interfaces: func() ([]net.Interface, error) {
			return []net.Interface{{Name: "lan0"}, {Name: "wan0"}}, nil
		},
		interfaceByName: func(name string) (*net.Interface, error) { return &net.Interface{Name: name}, nil },
		interfaceAddrs: func(iface *net.Interface) ([]net.Addr, error) {
			if iface.Name == "lan0" {
				return []net.Addr{&net.IPNet{IP: net.ParseIP(cfg.Gateway.LANIP), Mask: net.CIDRMask(24, 32)}}, nil
			}
			return nil, nil
		},
		now: time.Now,
	}}
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopIndex, startIndex := -1, -1
	for index, event := range events {
		if event == "dhcp-stop" && stopIndex == -1 {
			stopIndex = index
		}
		if event == "mihomo-start" && startIndex == -1 {
			startIndex = index
		}
	}
	if stopIndex == -1 || startIndex == -1 || stopIndex >= startIndex {
		t.Fatalf("reload events=%v", events)
	}
	state, exists, err := runtime.LoadState(paths.StateFile)
	profileDigest, digestErr := config.MihomoProfileDigest(cfg)
	if digestErr != nil {
		t.Fatal(digestErr)
	}
	if err != nil || !exists || state.PIDDNSMasq != 21 || state.PIDMihomo != 22 || state.ProfileDigest != profileDigest {
		t.Fatalf("fresh runtime state=%#v exists=%v err=%v", state, exists, err)
	}
}

func TestRestartMihomoValidatesBeforeStoppingLiveProcess(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	original := runtime.State{PIDDNSMasq: 11, PIDMihomo: 12, PFAnchorLoaded: true, StartedAt: time.Now()}
	if err := runtime.SaveState(paths.StateFile, original); err != nil {
		t.Fatal(err)
	}
	mihomoManager := &fakeMihomo{validateErr: errors.New("invalid prepared config")}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, saveState: runtime.SaveState,
		newMihomo: func(config.Config, runtime.Paths) mihomoService { return mihomoManager }, now: time.Now,
	}}

	err := manager.RestartMihomo(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid prepared config") {
		t.Fatalf("RestartMihomo() error=%v", err)
	}
	if mihomoManager.stopCalled || mihomoManager.startCalled {
		t.Fatal("restart touched the live process before prepared config validation passed")
	}
	state, exists, loadErr := runtime.LoadState(paths.StateFile)
	if loadErr != nil || !exists || state.PIDMihomo != original.PIDMihomo || state.PIDDNSMasq != original.PIDDNSMasq {
		t.Fatalf("runtime state=%#v exists=%v err=%v", state, exists, loadErr)
	}
}

func TestRestartMihomoRejectsImportedProfileDrift(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = filepath.Join(cfg.Runtime.Dir, "imported.yaml")
	if err := os.WriteFile(cfg.Mihomo.Profile, []byte("proxies: []\nproxy-groups: []\nrules: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{PIDDNSMasq: 11, PIDMihomo: 12, ProfileDigest: "older-applied-digest", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	mihomoManager := &fakeMihomo{}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, saveState: runtime.SaveState,
		newMihomo: func(config.Config, runtime.Paths) mihomoService { return mihomoManager }, now: time.Now,
	}}

	err := manager.RestartMihomo(context.Background())
	if err == nil || !strings.Contains(err.Error(), "differs from the applied runtime") {
		t.Fatalf("RestartMihomo() error=%v", err)
	}
	if mihomoManager.stopCalled || mihomoManager.startCalled {
		t.Fatal("restart touched mihomo while desired imported profile was not applied")
	}
}

func TestRestartMihomoReplacesOnlyProxyEngineAndArchivesLog(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	original := runtime.State{PIDDNSMasq: 11, PIDMihomo: 12, PFAnchorLoaded: true, IPForwardingBefore: "0", StartedAt: time.Now()}
	if err := runtime.SaveState(paths.StateFile, original); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.MihomoLog, []byte("link-down evidence\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	restartedAt := time.Date(2026, 7, 16, 12, 53, 33, 123456789, time.UTC)
	mihomoManager := &fakeMihomo{startPID: 22}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, saveState: runtime.SaveState,
		newMihomo: func(config.Config, runtime.Paths) mihomoService { return mihomoManager }, processFingerprint: fakeProcessFingerprint, now: func() time.Time { return restartedAt },
	}}

	if err := manager.RestartMihomo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !mihomoManager.stopCalled || mihomoManager.stoppedPID != original.PIDMihomo || !mihomoManager.startCalled {
		t.Fatalf("mihomo calls stop=%v stoppedPID=%d start=%v", mihomoManager.stopCalled, mihomoManager.stoppedPID, mihomoManager.startCalled)
	}
	state, exists, err := runtime.LoadState(paths.StateFile)
	if err != nil || !exists || state.PIDMihomo != 22 || state.PIDDNSMasq != original.PIDDNSMasq || !state.PFAnchorLoaded || state.IPForwardingBefore != original.IPForwardingBefore {
		t.Fatalf("runtime state=%#v exists=%v err=%v", state, exists, err)
	}
	archive := filepath.Join(paths.LogDir, "mihomo-before-restart-20260716T125333.123456789Z.log")
	data, err := os.ReadFile(archive)
	if err != nil || string(data) != "link-down evidence\n" {
		t.Fatalf("archived log=%q err=%v", data, err)
	}
}

func TestRestartMihomoStartFailureLeavesRetryableRuntimeState(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	snapshot := &runtime.SystemProxySnapshot{NetworkService: "Wi-Fi", Interface: "en0"}
	if err := runtime.SaveState(paths.StateFile, runtime.State{PIDDNSMasq: 11, PIDMihomo: 12, LocalSystemProxy: snapshot, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.MihomoLog, []byte("incident\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	mihomoManager := &fakeMihomo{startErr: errors.New("replacement failed")}
	systemProxyManager := &fakeLocalSystemProxy{}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, saveState: runtime.SaveState,
		newMihomo:           func(config.Config, runtime.Paths) mihomoService { return mihomoManager },
		newLocalSystemProxy: func() localSystemProxyService { return systemProxyManager }, now: time.Now,
	}}

	err := manager.RestartMihomo(context.Background())
	if err == nil || !strings.Contains(err.Error(), "replacement failed") {
		t.Fatalf("RestartMihomo() error=%v", err)
	}
	state, exists, loadErr := runtime.LoadState(paths.StateFile)
	if loadErr != nil || !exists || state.PIDMihomo != 0 || state.PIDDNSMasq != 11 {
		t.Fatalf("retryable runtime state=%#v exists=%v err=%v", state, exists, loadErr)
	}
	matches, globErr := filepath.Glob(filepath.Join(paths.LogDir, "mihomo-before-restart-*.log"))
	if globErr != nil || len(matches) != 1 {
		t.Fatalf("archived logs=%v err=%v", matches, globErr)
	}
	if systemProxyManager.restoreCalls != 1 {
		t.Fatalf("system proxy restore calls=%d, want 1", systemProxyManager.restoreCalls)
	}
}

func TestRestartMihomoStopFailureRestoresLivePID(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{PIDDNSMasq: 11, PIDMihomo: 12, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	mihomoManager := &fakeMihomo{stopErr: errors.New("old process is busy"), running: true}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, saveState: runtime.SaveState,
		newMihomo: func(config.Config, runtime.Paths) mihomoService { return mihomoManager }, now: time.Now,
	}}

	err := manager.RestartMihomo(context.Background())
	if err == nil || !strings.Contains(err.Error(), "old process is busy") {
		t.Fatalf("RestartMihomo() error=%v", err)
	}
	state, exists, loadErr := runtime.LoadState(paths.StateFile)
	if loadErr != nil || !exists || state.PIDMihomo != 12 || state.PIDDNSMasq != 11 {
		t.Fatalf("restored runtime state=%#v exists=%v err=%v", state, exists, loadErr)
	}
	if mihomoManager.startCalled {
		t.Fatal("replacement started while the old process was still alive")
	}
}

func TestStopFailureRetainsRuntimeStateForRetryAndRecovery(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{PIDDNSMasq: 11, PIDMihomo: 12, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, removeState: runtime.RemoveState,
		newDHCP: func(config.Config, runtime.Paths) dhcpService {
			return &fakeDHCP{stopErr: errors.New("dnsmasq did not stop")}
		},
		newMihomo: func(config.Config, runtime.Paths) mihomoService { return &fakeMihomo{} },
		newPF:     func(config.Config, runtime.Paths) pfService { return &fakePF{} },
		newSysctl: func() sysctlService { return &fakeSysctl{} },
	}}
	if err := manager.Stop(context.Background()); err == nil || !strings.Contains(err.Error(), "dnsmasq did not stop") {
		t.Fatalf("Stop() error=%v", err)
	}
	if _, exists, err := runtime.LoadState(paths.StateFile); err != nil || !exists {
		t.Fatalf("runtime state exists=%v err=%v", exists, err)
	}
}

func TestStopClearsPreviousBootRuntimeWithoutTouchingStalePIDsOrKernelState(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	snapshot := &runtime.SystemProxySnapshot{NetworkService: "Wi-Fi", Interface: "en0"}
	bootedAt := time.Now().Add(-30 * time.Minute)
	if err := runtime.SaveState(paths.StateFile, runtime.State{
		PIDDNSMasq:         11,
		PIDMihomo:          12,
		PFAnchorLoaded:     true,
		IPForwardingBefore: "0",
		LocalSystemProxy:   snapshot,
		StartedAt:          bootedAt.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.DevicePolicyApplied, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dhcpManager := &fakeDHCP{}
	mihomoManager := &fakeMihomo{}
	pfManager := &fakePF{}
	sysctlManager := &fakeSysctl{}
	systemProxyManager := &fakeLocalSystemProxy{}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, removeState: runtime.RemoveState,
		newDHCP:             func(config.Config, runtime.Paths) dhcpService { return dhcpManager },
		newMihomo:           func(config.Config, runtime.Paths) mihomoService { return mihomoManager },
		newPF:               func(config.Config, runtime.Paths) pfService { return pfManager },
		newSysctl:           func() sysctlService { return sysctlManager },
		newLocalSystemProxy: func() localSystemProxyService { return systemProxyManager },
		currentBoot: func() (runtime.BootSession, error) {
			return runtime.BootSession{ID: "current-boot", StartedAt: bootedAt}, nil
		},
	}}

	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if dhcpManager.stopCalled || mihomoManager.stopCalled || pfManager.unloadCalled || sysctlManager.restoreValue != "" {
		t.Fatalf("stale cleanup touched runtime pieces: dhcp=%v mihomo=%v pf=%v forwarding=%q", dhcpManager.stopCalled, mihomoManager.stopCalled, pfManager.unloadCalled, sysctlManager.restoreValue)
	}
	if systemProxyManager.restoreCalls != 1 {
		t.Fatalf("proxy restore calls = %d", systemProxyManager.restoreCalls)
	}
	if _, exists, err := runtime.LoadState(paths.StateFile); err != nil || exists {
		t.Fatalf("runtime state exists=%v err=%v", exists, err)
	}
	if _, err := os.Stat(paths.DevicePolicyApplied); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("applied policy snapshot still exists: %v", err)
	}
}

func TestStopKeepsPreviousBootRuntimeWhenSystemProxyRestoreFails(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	snapshot := &runtime.SystemProxySnapshot{NetworkService: "Wi-Fi", Interface: "en0"}
	if err := runtime.SaveState(paths.StateFile, runtime.State{
		LocalSystemProxy: snapshot,
		BootSessionID:    "previous-boot",
		StartedAt:        time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	systemProxyManager := &fakeLocalSystemProxy{restoreErr: errors.New("networksetup failed")}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid:             func() int { return 0 },
		loadState:           runtime.LoadState,
		removeState:         runtime.RemoveState,
		newLocalSystemProxy: func() localSystemProxyService { return systemProxyManager },
		currentBoot:         func() (runtime.BootSession, error) { return runtime.BootSession{ID: "current-boot"}, nil },
	}}

	err := manager.Stop(context.Background())
	if err == nil || !strings.Contains(err.Error(), "networksetup failed") {
		t.Fatalf("Stop() error = %v", err)
	}
	if _, exists, err := runtime.LoadState(paths.StateFile); err != nil || !exists {
		t.Fatalf("runtime state exists=%v err=%v", exists, err)
	}
}

func TestStopDoesNotSignalCurrentBootPIDWithDifferentFingerprint(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{
		PIDDNSMasq:                11,
		DNSMasqProcessFingerprint: "dnsmasq-original",
		PIDMihomo:                 12,
		MihomoProcessFingerprint:  "mihomo-original",
		BootSessionID:             "current-boot",
		StartedAt:                 time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	dhcpManager := &fakeDHCP{}
	mihomoManager := &fakeMihomo{}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, removeState: runtime.RemoveState,
		newDHCP:        func(config.Config, runtime.Paths) dhcpService { return dhcpManager },
		newMihomo:      func(config.Config, runtime.Paths) mihomoService { return mihomoManager },
		newPF:          func(config.Config, runtime.Paths) pfService { return &fakePF{} },
		newSysctl:      func() sysctlService { return &fakeSysctl{} },
		currentBoot:    func() (runtime.BootSession, error) { return runtime.BootSession{ID: "current-boot"}, nil },
		processMatches: func(int, string) (bool, error) { return false, nil },
	}}

	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if dhcpManager.stopCalled || mihomoManager.stopCalled {
		t.Fatalf("reused PID was signaled: dhcp=%v mihomo=%v", dhcpManager.stopCalled, mihomoManager.stopCalled)
	}
}

func TestRestartMihomoRejectsPreviousBootRuntimeBeforeProcessValidation(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(paths.StateFile, runtime.State{PIDMihomo: 12, BootSessionID: "previous-boot", StartedAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	mihomoManager := &fakeMihomo{}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState,
		newMihomo:   func(config.Config, runtime.Paths) mihomoService { return mihomoManager },
		currentBoot: func() (runtime.BootSession, error) { return runtime.BootSession{ID: "current-boot"}, nil },
	}}

	err := manager.RestartMihomo(context.Background())
	if err == nil || !strings.Contains(err.Error(), "interrupted by a system reboot") {
		t.Fatalf("RestartMihomo() error = %v", err)
	}
	if mihomoManager.validateCalled || mihomoManager.stopCalled || mihomoManager.startCalled {
		t.Fatal("restart touched mihomo for a previous-boot runtime")
	}
}

func TestStopProxyRestoreFailureKeepsServicesRunningAndStateRetryable(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	paths := runtime.NewPaths(cfg)
	if err := runtime.Ensure(paths); err != nil {
		t.Fatal(err)
	}
	snapshot := &runtime.SystemProxySnapshot{NetworkService: "Wi-Fi", Interface: "en0"}
	if err := runtime.SaveState(paths.StateFile, runtime.State{PIDDNSMasq: 11, PIDMihomo: 12, LocalSystemProxy: snapshot, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	dhcpManager := &fakeDHCP{}
	mihomoManager := &fakeMihomo{}
	systemProxyManager := &fakeLocalSystemProxy{restoreErr: errors.New("networksetup failed")}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, removeState: runtime.RemoveState,
		newDHCP:             func(config.Config, runtime.Paths) dhcpService { return dhcpManager },
		newMihomo:           func(config.Config, runtime.Paths) mihomoService { return mihomoManager },
		newPF:               func(config.Config, runtime.Paths) pfService { return &fakePF{} },
		newSysctl:           func() sysctlService { return &fakeSysctl{} },
		newLocalSystemProxy: func() localSystemProxyService { return systemProxyManager },
	}}

	err := manager.Stop(context.Background())
	if err == nil || !strings.Contains(err.Error(), "networksetup failed") {
		t.Fatalf("Stop() error=%v", err)
	}
	if dhcpManager.stopCalled || mihomoManager.stopCalled {
		t.Fatal("gateway services stopped even though the system proxy could not be restored")
	}
	if _, exists, err := runtime.LoadState(paths.StateFile); err != nil || !exists {
		t.Fatalf("runtime state exists=%v err=%v", exists, err)
	}
}

func TestIPv6TakeoverLifecycleOrdersMihomoBrokerRAAndDNSMasq(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "lan0"
	cfg.Gateway.UpstreamInterface = "wan0"
	cfg.Gateway.LANIP = "192.168.50.1"
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Always
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	events := []string{}
	dhcpManager := &fakeDHCP{startPID: 11, events: &events}
	mihomoManager := &fakeMihomo{startPID: 12, events: &events}
	packetManager := &fakeIPv6Packet{startPID: 13, events: &events}
	hostManager := &fakeIPv6Host{events: &events}
	pfManager := &fakePF{loaded: true, events: &events}
	var mihomoConfigs []config.Config
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, saveState: runtime.SaveState,
		removeState: runtime.RemoveState, ensure: runtime.Ensure,
		newDHCP: func(config.Config, runtime.Paths) dhcpService { return dhcpManager },
		newMihomo: func(cfg config.Config, _ runtime.Paths) mihomoService {
			mihomoConfigs = append(mihomoConfigs, cfg)
			return mihomoManager
		},
		newPF:              func(config.Config, runtime.Paths) pfService { return pfManager },
		newSysctl:          func() sysctlService { return &fakeSysctl{current: "0"} },
		newIPv6Packet:      func(config.Config, runtime.Paths) ipv6PacketService { return packetManager },
		newIPv6Host:        func(config.Config) ipv6HostService { return hostManager },
		currentBoot:        func() (runtime.BootSession, error) { return runtime.BootSession{ID: "boot-a"}, nil },
		processFingerprint: fakeProcessFingerprint,
		processMatches:     fakeProcessMatches,
		interfaces: func() ([]net.Interface, error) {
			return []net.Interface{{Name: "lan0"}, {Name: "wan0"}}, nil
		},
		interfaceByName: func(name string) (*net.Interface, error) { return &net.Interface{Name: name}, nil },
		interfaceAddrs: func(iface *net.Interface) ([]net.Addr, error) {
			if iface.Name == "lan0" {
				return []net.Addr{&net.IPNet{IP: net.ParseIP(cfg.Gateway.LANIP), Mask: net.CIDRMask(24, 32)}}, nil
			}
			return nil, nil
		},
		now: time.Now,
	}}

	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, exists, err := runtime.LoadState(paths.StateFile)
	if err != nil || !exists || !state.IPv6PacketEffective || !state.IPv6GatewayAliasOwned || state.PIDIPv6Packet != 13 || state.TUNIPv6Requested != config.TUNIPv6Always {
		t.Fatalf("IPv6 runtime state = %#v, exists=%v, err=%v", state, exists, err)
	}
	assertEventOrder(t, events, "mihomo-start", "ipv6-packet-start", "ipv6-gateway-add", "dhcp-start")

	manager.cfg.Transparent.TUNIPv6 = config.TUNIPv6Off
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := mihomoConfigs[len(mihomoConfigs)-1].Transparent.TUNIPv6; got != config.TUNIPv6Always {
		t.Fatalf("stop used desired IPv6 mode %q instead of applied mode", got)
	}
	assertEventOrder(t, events, "dhcp-stop", "ipv6-withdraw", "ipv6-gateway-remove", "ipv6-packet-stop", "mihomo-stop")
	if _, exists, err := runtime.LoadState(paths.StateFile); err != nil || exists {
		t.Fatalf("runtime state after stop exists=%v err=%v", exists, err)
	}
}

func TestIPv6GatewayAddFailureRollsBackOwnedAliasAndPacketBroker(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "lan0"
	cfg.Gateway.UpstreamInterface = "wan0"
	cfg.Gateway.LANIP = "192.168.50.1"
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Always
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	paths := runtime.NewPaths(cfg)
	events := []string{}
	dhcpManager := &fakeDHCP{startPID: 11, events: &events}
	mihomoManager := &fakeMihomo{startPID: 12, events: &events}
	packetManager := &fakeIPv6Packet{startPID: 13, events: &events}
	hostManager := &fakeIPv6Host{addErr: errors.New("IPv6 duplicate address detection failed"), events: &events}
	pfManager := &fakePF{loaded: true, events: &events}
	manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, saveState: runtime.SaveState,
		removeState: runtime.RemoveState, ensure: runtime.Ensure,
		newDHCP:            func(config.Config, runtime.Paths) dhcpService { return dhcpManager },
		newMihomo:          func(config.Config, runtime.Paths) mihomoService { return mihomoManager },
		newPF:              func(config.Config, runtime.Paths) pfService { return pfManager },
		newSysctl:          func() sysctlService { return &fakeSysctl{current: "0"} },
		newIPv6Packet:      func(config.Config, runtime.Paths) ipv6PacketService { return packetManager },
		newIPv6Host:        func(config.Config) ipv6HostService { return hostManager },
		currentBoot:        func() (runtime.BootSession, error) { return runtime.BootSession{ID: "boot-a"}, nil },
		processFingerprint: fakeProcessFingerprint,
		processMatches:     fakeProcessMatches,
		interfaces: func() ([]net.Interface, error) {
			return []net.Interface{{Name: "lan0"}, {Name: "wan0"}}, nil
		},
		interfaceByName: func(name string) (*net.Interface, error) { return &net.Interface{Name: name}, nil },
		interfaceAddrs: func(iface *net.Interface) ([]net.Addr, error) {
			if iface.Name == "lan0" {
				return []net.Addr{&net.IPNet{IP: net.ParseIP(cfg.Gateway.LANIP), Mask: net.CIDRMask(24, 32)}}, nil
			}
			return nil, nil
		},
		now: time.Now,
	}}

	err := manager.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "IPv6 duplicate address detection failed") {
		t.Fatalf("Start() error = %v", err)
	}
	if dhcpManager.startCalled {
		t.Fatal("dnsmasq started after the IPv6 gateway alias failed")
	}
	assertEventOrder(t, events,
		"mihomo-start",
		"ipv6-packet-start",
		"ipv6-gateway-add",
		"ipv6-withdraw",
		"ipv6-gateway-remove",
		"ipv6-packet-stop",
		"mihomo-stop",
	)
	if _, exists, loadErr := runtime.LoadState(paths.StateFile); loadErr != nil || exists {
		t.Fatalf("runtime state after rollback exists=%v err=%v", exists, loadErr)
	}
}

func TestSameLANIPv6CleanupDoesNotBroadcastRouterWithdrawal(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Mode = config.GatewayModeSameLAN
	cfg.Gateway.UpstreamInterface = cfg.Gateway.Interface
	cfg.DHCP.Enabled = false
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Always
	cfg.Transparent.IPv6SharedL2Ready = true
	events := []string{}
	host := &fakeIPv6Host{events: &events}
	manager := Manager{cfg: cfg, paths: runtime.NewPaths(cfg)}
	state := runtime.State{
		TUNIPv6Requested:      config.TUNIPv6Always,
		IPv6PacketEffective:   true,
		IPv6GatewayAliasOwned: true,
		IPv6RAEffective:       false,
	}
	deps := gatewayDeps{newIPv6Host: func(config.Config) ipv6HostService { return host }}
	if err := manager.cleanupIPv6(context.Background(), deps, state); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(events, "ipv6-withdraw") {
		t.Fatalf("same-LAN selective IPv6 cleanup broadcast a withdrawal: %v", events)
	}
	if !slices.Contains(events, "ipv6-gateway-remove") {
		t.Fatalf("same-LAN selective IPv6 cleanup did not remove the gateway alias: %v", events)
	}
}

func TestResolveIPv6AutoFailsClosedWithoutNativeUpstream(t *testing.T) {
	cfg := config.Default()
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Auto
	host := &fakeIPv6Host{native: false}
	manager := Manager{cfg: cfg}
	resolution := manager.resolveIPv6(gatewayDeps{newIPv6Host: func(config.Config) ipv6HostService { return host }})
	if resolution.Requested != config.TUNIPv6Auto || resolution.Effective || resolution.Reason != "native_ipv6_unavailable" || manager.cfg.Transparent.TUNIPv6 != config.TUNIPv6Off {
		t.Fatalf("auto resolution = %#v, effective config=%q", resolution, manager.cfg.Transparent.TUNIPv6)
	}
}

func TestResolveIPv6AlwaysReportsNativeStateWithoutFailingClosed(t *testing.T) {
	cfg := config.Default()
	cfg.Transparent.Mode = config.TransparentModeTUN
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Always
	host := &fakeIPv6Host{native: true}
	manager := Manager{cfg: cfg}
	resolution := manager.resolveIPv6(gatewayDeps{newIPv6Host: func(config.Config) ipv6HostService { return host }})
	if !resolution.Effective || !resolution.NativeAvailable || resolution.Reason != "forced_userspace_packet_path" || manager.cfg.Transparent.TUNIPv6 != config.TUNIPv6Always {
		t.Fatalf("always resolution = %#v, effective config=%q", resolution, manager.cfg.Transparent.TUNIPv6)
	}

	host.native = false
	host.nativeErr = errors.New("route probe failed")
	resolution = manager.resolveIPv6(gatewayDeps{newIPv6Host: func(config.Config) ipv6HostService { return host }})
	if !resolution.Effective || resolution.NativeAvailable || !strings.Contains(resolution.Reason, "native_detection_failed: route probe failed") || manager.cfg.Transparent.TUNIPv6 != config.TUNIPv6Always {
		t.Fatalf("always detection-error resolution = %#v, effective config=%q", resolution, manager.cfg.Transparent.TUNIPv6)
	}
}

type fakeDHCP struct {
	checkErr    error
	writeErr    error
	startPID    int
	startErr    error
	stopErr     error
	startCalled bool
	stopCalled  bool
	running     bool
	events      *[]string
}

func (f *fakeDHCP) Check() error {
	return f.checkErr
}

func (f *fakeDHCP) WriteConfig() error {
	if f.events != nil {
		*f.events = append(*f.events, "dhcp-write")
	}
	return f.writeErr
}

func (f *fakeDHCP) Start() (int, error) {
	f.startCalled = true
	if f.events != nil {
		*f.events = append(*f.events, "dhcp-start")
	}
	return f.startPID, f.startErr
}

func (f *fakeDHCP) Stop(int) error {
	f.stopCalled = true
	if f.events != nil {
		*f.events = append(*f.events, "dhcp-stop")
	}
	return f.stopErr
}

func (f *fakeDHCP) Running(int) bool { return f.running }

type fakeMihomo struct {
	checkErr       error
	writeErr       error
	validateErr    error
	startPID       int
	startErr       error
	stopErr        error
	startCalled    bool
	stopCalled     bool
	validateCalled bool
	stoppedPID     int
	running        bool
	events         *[]string
}

func (f *fakeMihomo) Check() error {
	return f.checkErr
}

func (f *fakeMihomo) WriteConfig() error {
	if f.events != nil {
		*f.events = append(*f.events, "mihomo-write")
	}
	return f.writeErr
}

func (f *fakeMihomo) ValidateWrittenConfig() error {
	f.validateCalled = true
	if f.events != nil {
		*f.events = append(*f.events, "mihomo-validate")
	}
	return f.validateErr
}

func (f *fakeMihomo) Start() (int, error) {
	f.startCalled = true
	if f.events != nil {
		*f.events = append(*f.events, "mihomo-start")
	}
	return f.startPID, f.startErr
}

func (f *fakeMihomo) Stop(pid int) error {
	f.stopCalled = true
	f.stoppedPID = pid
	if f.events != nil {
		*f.events = append(*f.events, "mihomo-stop")
	}
	return f.stopErr
}

func (f *fakeMihomo) Running(int) bool { return f.running }

type fakePF struct {
	checkErr     error
	writeErr     error
	enabled      bool
	enabledErr   error
	loadErr      error
	loaded       bool
	loadedErr    error
	unloadErr    error
	loadCalled   bool
	unloadCalled bool
	events       *[]string
}

func (f *fakePF) Check() error {
	return f.checkErr
}

func (f *fakePF) WriteAnchor() error {
	return f.writeErr
}

func (f *fakePF) Enabled() (bool, error) {
	return f.enabled, f.enabledErr
}

func (f *fakePF) Load(bool) error {
	f.loadCalled = true
	if f.events != nil {
		*f.events = append(*f.events, "pf-load")
	}
	return f.loadErr
}

func (f *fakePF) Loaded() (bool, error) {
	return f.loaded, f.loadedErr
}

func (f *fakePF) Unload(bool) error {
	f.unloadCalled = true
	return f.unloadErr
}

type fakeSysctl struct {
	checkErr     error
	current      string
	currentErr   error
	enableErr    error
	restoreErr   error
	enableCalled bool
	restoreValue string
}

func (f *fakeSysctl) Check() error {
	return f.checkErr
}

func (f *fakeSysctl) Current() (string, error) {
	return f.current, f.currentErr
}

func (f *fakeSysctl) Enable() error {
	f.enableCalled = true
	return f.enableErr
}

func (f *fakeSysctl) Restore(value string) error {
	f.restoreValue = value
	return f.restoreErr
}

type fakeLocalSystemProxy struct {
	snapshot         runtime.SystemProxySnapshot
	prepareErr       error
	enableErr        error
	restoreErr       error
	prepareInterface string
	preparePort      int
	enableCalls      int
	restoreCalls     int
	events           *[]string
}

type fakeIPv6Packet struct {
	checkErr error
	startPID int
	startErr error
	stopErr  error
	running  bool
	events   *[]string
}

func (f *fakeIPv6Packet) Check() error { return f.checkErr }
func (f *fakeIPv6Packet) Start() (int, error) {
	if f.events != nil {
		*f.events = append(*f.events, "ipv6-packet-start")
	}
	return f.startPID, f.startErr
}
func (f *fakeIPv6Packet) Stop(int) error {
	if f.events != nil {
		*f.events = append(*f.events, "ipv6-packet-stop")
	}
	return f.stopErr
}
func (f *fakeIPv6Packet) Running(int) bool { return f.running }

type fakeIPv6Host struct {
	native      bool
	nativeErr   error
	checkErr    error
	addErr      error
	removeErr   error
	withdrawErr error
	events      *[]string
}

func (f *fakeIPv6Host) NativeAvailable() (bool, error) { return f.native, f.nativeErr }
func (f *fakeIPv6Host) CheckGatewayAvailable() error   { return f.checkErr }
func (f *fakeIPv6Host) AddGateway(context.Context) error {
	if f.events != nil {
		*f.events = append(*f.events, "ipv6-gateway-add")
	}
	return f.addErr
}
func (f *fakeIPv6Host) RemoveGateway(context.Context) error {
	if f.events != nil {
		*f.events = append(*f.events, "ipv6-gateway-remove")
	}
	return f.removeErr
}
func (f *fakeIPv6Host) Withdraw(context.Context) error {
	if f.events != nil {
		*f.events = append(*f.events, "ipv6-withdraw")
	}
	return f.withdrawErr
}

func (f *fakeLocalSystemProxy) Prepare(_ context.Context, interfaceName string, port int) (runtime.SystemProxySnapshot, error) {
	f.prepareInterface = interfaceName
	f.preparePort = port
	if f.events != nil {
		*f.events = append(*f.events, "system-proxy-prepare")
	}
	return f.snapshot, f.prepareErr
}

func (f *fakeLocalSystemProxy) Enable(_ context.Context, _ runtime.SystemProxySnapshot, _ int) error {
	f.enableCalls++
	if f.events != nil {
		*f.events = append(*f.events, "system-proxy-enable")
	}
	return f.enableErr
}

func (f *fakeLocalSystemProxy) Restore(_ context.Context, _ runtime.SystemProxySnapshot) error {
	f.restoreCalls++
	if f.events != nil {
		*f.events = append(*f.events, "system-proxy-restore")
	}
	return f.restoreErr
}

func indexOfEvent(events []string, target string) int {
	for index, event := range events {
		if event == target {
			return index
		}
	}
	return -1
}

func assertEventOrder(t *testing.T, events []string, ordered ...string) {
	t.Helper()
	position := -1
	for _, event := range ordered {
		next := indexOfEvent(events, event)
		if next <= position {
			t.Fatalf("event %q did not occur after offset %d: %v", event, position, events)
		}
		position = next
	}
}

func fakeProcessFingerprint(pid int) (string, error) {
	if pid <= 0 {
		return "", nil
	}
	return fmt.Sprintf("pid-%d-start", pid), nil
}

func fakeProcessMatches(pid int, fingerprint string) (bool, error) {
	expected, _ := fakeProcessFingerprint(pid)
	return expected != "" && fingerprint == expected, nil
}
