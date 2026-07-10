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

type fakeDHCP struct {
	checkErr    error
	writeErr    error
	startPID    int
	startErr    error
	stopErr     error
	startCalled bool
	stopCalled  bool
}

func (f *fakeDHCP) Check() error {
	return f.checkErr
}

func (f *fakeDHCP) WriteConfig() error {
	return f.writeErr
}

func (f *fakeDHCP) Start() (int, error) {
	f.startCalled = true
	return f.startPID, f.startErr
}

func (f *fakeDHCP) Stop(int) error {
	f.stopCalled = true
	return f.stopErr
}

type fakeMihomo struct {
	checkErr   error
	writeErr   error
	startPID   int
	startErr   error
	stopErr    error
	stopCalled bool
}

func (f *fakeMihomo) Check() error {
	return f.checkErr
}

func (f *fakeMihomo) WriteConfig() error {
	return f.writeErr
}

func (f *fakeMihomo) Start() (int, error) {
	return f.startPID, f.startErr
}

func (f *fakeMihomo) Stop(int) error {
	f.stopCalled = true
	return f.stopErr
}

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
