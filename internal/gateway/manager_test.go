package gateway

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/runtime"
)

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
		newDHCP:   func(config.Config, runtime.Paths) dhcpService { return dhcpManager },
		newMihomo: func(config.Config, runtime.Paths) mihomoService { return mihomoManager },
		newPF:     func(config.Config, runtime.Paths) pfService { return &fakePF{loaded: true} },
		newSysctl: func() sysctlService { return &fakeSysctl{current: "0"} },
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
	checkErr    error
	writeErr    error
	validateErr error
	startPID    int
	startErr    error
	stopErr     error
	stopCalled  bool
	running     bool
	events      *[]string
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
	if f.events != nil {
		*f.events = append(*f.events, "mihomo-validate")
	}
	return f.validateErr
}

func (f *fakeMihomo) Start() (int, error) {
	if f.events != nil {
		*f.events = append(*f.events, "mihomo-start")
	}
	return f.startPID, f.startErr
}

func (f *fakeMihomo) Stop(int) error {
	f.stopCalled = true
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
