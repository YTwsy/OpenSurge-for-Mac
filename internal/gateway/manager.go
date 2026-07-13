package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/dhcp"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/pf"
	"open-mihomo-gateway/internal/runtime"
	"open-mihomo-gateway/internal/sysctl"
)

type Manager struct {
	cfg   config.Config
	paths runtime.Paths
	deps  gatewayDeps
}

func New(cfg config.Config) Manager {
	return Manager{cfg: cfg, paths: runtime.NewPaths(cfg), deps: defaultGatewayDeps()}
}

type dhcpService interface {
	Check() error
	WriteConfig() error
	Start() (int, error)
	Stop(int) error
	Running(int) bool
}

type mihomoService interface {
	Check() error
	WriteConfig() error
	ValidateWrittenConfig() error
	Start() (int, error)
	Stop(int) error
	Running(int) bool
}

type pfService interface {
	Check() error
	WriteAnchor() error
	Enabled() (bool, error)
	Load(bool) error
	Loaded() (bool, error)
	Unload(bool) error
}

type sysctlService interface {
	Check() error
	Current() (string, error)
	Enable() error
	Restore(string) error
}

type gatewayDeps struct {
	geteuid            func() int
	loadState          func(string) (runtime.State, bool, error)
	saveState          func(string, runtime.State) error
	removeState        func(string) error
	ensure             func(runtime.Paths) error
	newDHCP            func(config.Config, runtime.Paths) dhcpService
	newMihomo          func(config.Config, runtime.Paths) mihomoService
	newPF              func(config.Config, runtime.Paths) pfService
	newSysctl          func() sysctlService
	interfaces         func() ([]net.Interface, error)
	interfaceByName    func(string) (*net.Interface, error)
	interfaceAddrs     func(*net.Interface) ([]net.Addr, error)
	probeReservationIP func(ip string, expectedMAC string) error
	now                func() time.Time
}

func defaultGatewayDeps() gatewayDeps {
	return gatewayDeps{
		geteuid:     os.Geteuid,
		loadState:   runtime.LoadState,
		saveState:   runtime.SaveState,
		removeState: runtime.RemoveState,
		ensure:      runtime.Ensure,
		newDHCP: func(cfg config.Config, paths runtime.Paths) dhcpService {
			return dhcp.New(cfg, paths)
		},
		newMihomo: func(cfg config.Config, paths runtime.Paths) mihomoService {
			return mihomo.New(cfg, paths)
		},
		newPF: func(cfg config.Config, paths runtime.Paths) pfService {
			return pf.New(cfg, paths)
		},
		newSysctl: func() sysctlService {
			return sysctl.New()
		},
		interfaces:      net.Interfaces,
		interfaceByName: net.InterfaceByName,
		interfaceAddrs: func(iface *net.Interface) ([]net.Addr, error) {
			return iface.Addrs()
		},
		probeReservationIP: probeReservationIPConflict,
		now:                time.Now,
	}
}

func (m Manager) gatewayDeps() gatewayDeps {
	if m.deps.geteuid == nil {
		return defaultGatewayDeps()
	}
	return m.deps
}

func (m Manager) Start(_ context.Context) error {
	deps := m.gatewayDeps()
	if deps.geteuid() != 0 {
		return fmt.Errorf("start requires sudo/root privileges")
	}
	if _, exists, err := deps.loadState(m.paths.StateFile); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("gateway state already exists; run stop first")
	}
	if err := config.PrepareDevicePolicy(&m.cfg); err != nil {
		return err
	}
	if err := config.Validate(m.cfg); err != nil {
		return err
	}
	if err := deps.ensure(m.paths); err != nil {
		return err
	}

	dhcpManager := deps.newDHCP(m.cfg, m.paths)
	mihomoManager := deps.newMihomo(m.cfg, m.paths)
	pfManager := deps.newPF(m.cfg, m.paths)
	sysctlManager := deps.newSysctl()
	if err := m.preflight(dhcpManager, mihomoManager, pfManager, sysctlManager, deps); err != nil {
		return err
	}
	if err := m.checkReservationConflicts(deps); err != nil {
		return err
	}
	if err := mihomoManager.WriteConfig(); err != nil {
		return err
	}
	if err := dhcpManager.WriteConfig(); err != nil {
		return err
	}
	if err := pfManager.WriteAnchor(); err != nil {
		return err
	}
	if err := mihomoManager.ValidateWrittenConfig(); err != nil {
		return err
	}
	if bundle := m.cfg.DevicePolicy.Bundle; bundle != nil {
		if err := dhcp.ReconcilePolicyLeases(m.paths.LeaseFile, bundle.Compiled.Reservations); err != nil {
			return err
		}
		if err := device.WritePolicyBundleSnapshot(m.paths.DevicePolicyApplied, *bundle); err != nil {
			return err
		}
	}

	ipForwardingBefore, err := sysctlManager.Current()
	if err != nil {
		return err
	}
	pfEnabledBefore, err := pfManager.Enabled()
	if err != nil {
		return err
	}

	state := runtime.State{
		StartedAt:          deps.now(),
		IPForwardingBefore: ipForwardingBefore,
		PFEnabledBefore:    pfEnabledBefore,
	}
	if bundle := m.cfg.DevicePolicy.Bundle; bundle != nil {
		state.DevicePolicyDigest = bundle.Digest
	}
	if err := deps.saveState(m.paths.StateFile, state); err != nil {
		_ = device.RemovePolicyBundleSnapshot(m.paths.DevicePolicyApplied)
		return err
	}

	if err := sysctlManager.Enable(); err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}

	mihomoPID, err := mihomoManager.Start()
	if err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}
	state.PIDMihomo = mihomoPID
	if err := deps.saveState(m.paths.StateFile, state); err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}

	pid, err := dhcpManager.Start()
	if err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}
	state.PIDDNSMasq = pid
	if err := deps.saveState(m.paths.StateFile, state); err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}

	if err := pfManager.Load(!pfEnabledBefore); err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}
	state.PFAnchorLoaded = true
	loaded, err := pfManager.Loaded()
	if err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}
	if !loaded {
		return m.rollback(fmt.Errorf("pf anchor %s did not become visible after load", m.cfg.PF.AnchorName), state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}
	if err := deps.saveState(m.paths.StateFile, state); err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}

	fmt.Printf("Gateway runtime prepared in %s\n", m.paths.Dir)
	if mihomoPID > 0 {
		fmt.Printf("mihomo started with pid %d\n", mihomoPID)
	}
	if pid > 0 {
		fmt.Printf("dnsmasq started with pid %d\n", pid)
	}
	fmt.Printf("pf anchor %s loaded\n", m.cfg.PF.AnchorName)
	return nil
}

// Reload validates a complete desired candidate before touching the running
// gateway, then performs the same audited stop/start lifecycle as the normal
// commands. The Manager owns an immutable Config value, so the configuration
// that passed validation is also the configuration applied after stop.
func (m Manager) Reload(ctx context.Context) error {
	deps := m.gatewayDeps()
	if deps.geteuid() != 0 {
		return fmt.Errorf("reload requires sudo/root privileges")
	}
	state, exists, err := deps.loadState(m.paths.StateFile)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("gateway is not running; run start instead")
	}
	if !deps.newDHCP(m.cfg, m.paths).Running(state.PIDDNSMasq) || !deps.newMihomo(m.cfg, m.paths).Running(state.PIDMihomo) {
		return fmt.Errorf("gateway is degraded; reload requires both DHCP/DNS and mihomo to be running")
	}
	if err := m.validateReloadCandidate(); err != nil {
		return fmt.Errorf("reload candidate validation failed: %w", err)
	}
	if err := m.Stop(ctx); err != nil {
		return fmt.Errorf("reload stop failed: %w", err)
	}
	if err := m.Start(ctx); err != nil {
		return fmt.Errorf("reload start failed after gateway stop: %w", err)
	}
	return nil
}

// validateReloadCandidate renders every generated artifact into an isolated
// temporary runtime and runs the real mihomo validator. It deliberately does
// not write applied policy state or alter host networking.
func (m Manager) validateReloadCandidate() error {
	parent := filepath.Dir(m.paths.Dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(parent, ".opensurge-reload-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)

	candidateConfig := m.cfg
	candidateConfig.Runtime.Dir = temp
	candidateConfig.Mihomo.Config = filepath.Join(temp, "mihomo.yaml")
	if err := config.PrepareDevicePolicy(&candidateConfig); err != nil {
		return err
	}
	if err := config.Validate(candidateConfig); err != nil {
		return err
	}
	candidate := Manager{cfg: candidateConfig, paths: runtime.NewPaths(candidateConfig), deps: m.gatewayDeps()}
	deps := candidate.gatewayDeps()
	if err := deps.ensure(candidate.paths); err != nil {
		return err
	}
	dhcpManager := deps.newDHCP(candidate.cfg, candidate.paths)
	mihomoManager := deps.newMihomo(candidate.cfg, candidate.paths)
	pfManager := deps.newPF(candidate.cfg, candidate.paths)
	sysctlManager := deps.newSysctl()
	if err := candidate.preflight(dhcpManager, mihomoManager, pfManager, sysctlManager, deps); err != nil {
		return err
	}
	if err := candidate.checkReservationConflicts(deps); err != nil {
		return err
	}
	if err := mihomoManager.WriteConfig(); err != nil {
		return err
	}
	if err := dhcpManager.WriteConfig(); err != nil {
		return err
	}
	if err := pfManager.WriteAnchor(); err != nil {
		return err
	}
	return mihomoManager.ValidateWrittenConfig()
}

func (m Manager) Stop(_ context.Context) error {
	deps := m.gatewayDeps()
	if deps.geteuid() != 0 {
		return fmt.Errorf("stop requires sudo/root privileges")
	}
	state, exists, err := deps.loadState(m.paths.StateFile)
	if err != nil {
		return err
	}
	var cleanupErr error
	pfManager := deps.newPF(m.cfg, m.paths)
	sysctlManager := deps.newSysctl()
	if exists {
		dhcpManager := deps.newDHCP(m.cfg, m.paths)
		cleanupErr = errors.Join(cleanupErr, dhcpManager.Stop(state.PIDDNSMasq))
		mihomoManager := deps.newMihomo(m.cfg, m.paths)
		cleanupErr = errors.Join(cleanupErr, mihomoManager.Stop(state.PIDMihomo))
		if state.PFAnchorLoaded {
			cleanupErr = errors.Join(cleanupErr, pfManager.Unload(!state.PFEnabledBefore))
		}
		cleanupErr = errors.Join(cleanupErr, sysctlManager.Restore(state.IPForwardingBefore))
	}
	if cleanupErr != nil {
		return cleanupErr
	}
	cleanupErr = errors.Join(cleanupErr, deps.removeState(m.paths.StateFile))
	cleanupErr = errors.Join(cleanupErr, device.RemovePolicyBundleSnapshot(m.paths.DevicePolicyApplied))
	if cleanupErr != nil {
		return cleanupErr
	}

	fmt.Println("Gateway stopped and runtime state cleared.")
	return nil
}

func (m Manager) preflight(dhcpManager dhcpService, mihomoManager mihomoService, pfManager pfService, sysctlManager sysctlService, deps gatewayDeps) error {
	if err := dhcpManager.Check(); err != nil {
		return err
	}
	if err := mihomoManager.Check(); err != nil {
		return err
	}
	if err := pfManager.Check(); err != nil {
		return err
	}
	if err := sysctlManager.Check(); err != nil {
		return err
	}
	sameInterface := strings.TrimSpace(m.cfg.Gateway.Interface) == strings.TrimSpace(m.cfg.Gateway.UpstreamInterface)
	if m.cfg.Gateway.SameLAN() {
		if !sameInterface {
			return fmt.Errorf("gateway.mode %s requires gateway and upstream interfaces to match", m.cfg.Gateway.Mode)
		}
	} else if sameInterface {
		return fmt.Errorf("gateway and upstream interfaces must differ")
	}
	if _, err := deps.interfaceByName(m.cfg.Gateway.Interface); err != nil {
		return fmt.Errorf("interface %s: %w", m.cfg.Gateway.Interface, err)
	}
	if _, err := deps.interfaceByName(m.cfg.Gateway.UpstreamInterface); err != nil {
		return fmt.Errorf("upstream interface %s: %w", m.cfg.Gateway.UpstreamInterface, err)
	}
	return m.checkLANIP(deps)
}

func (m Manager) checkLANIP(deps gatewayDeps) error {
	target := net.ParseIP(m.cfg.Gateway.LANIP).To4()
	if target == nil {
		return fmt.Errorf("gateway LAN IP %s is not IPv4", m.cfg.Gateway.LANIP)
	}
	iface, err := deps.interfaceByName(m.cfg.Gateway.Interface)
	if err != nil {
		return err
	}
	addrs, err := deps.interfaceAddrs(iface)
	if err != nil {
		return err
	}
	found := false
	for _, addr := range addrs {
		if addrHasIPv4(addr, target) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("LAN IP %s is not configured on interface %s", m.cfg.Gateway.LANIP, m.cfg.Gateway.Interface)
	}
	return m.checkLANIPConflicts(target, iface.Name, deps)
}

func (m Manager) checkLANIPConflicts(target net.IP, gatewayInterface string, deps gatewayDeps) error {
	interfaces := deps.interfaces
	if interfaces == nil {
		interfaces = net.Interfaces
	}
	ifaces, err := interfaces()
	if err != nil {
		return err
	}
	for _, candidate := range ifaces {
		if candidate.Name == gatewayInterface {
			continue
		}
		addrs, err := deps.interfaceAddrs(&candidate)
		if err != nil {
			return fmt.Errorf("interface %s addresses: %w", candidate.Name, err)
		}
		for _, addr := range addrs {
			if addrHasIPv4(addr, target) {
				return fmt.Errorf("LAN IP %s is also configured on interface %s; remove the duplicate address before starting the gateway", m.cfg.Gateway.LANIP, candidate.Name)
			}
		}
	}
	return nil
}

func addrHasIPv4(addr net.Addr, target net.IP) bool {
	switch value := addr.(type) {
	case *net.IPNet:
		return value.IP.To4() != nil && value.IP.Equal(target)
	case *net.IPAddr:
		return value.IP.To4() != nil && value.IP.Equal(target)
	default:
		return false
	}
}

func (m Manager) rollback(cause error, state runtime.State, dhcpManager dhcpService, mihomoManager mihomoService, pfManager pfService, sysctlManager sysctlService) error {
	deps := m.gatewayDeps()
	var cleanupErr error
	cleanupErr = errors.Join(cleanupErr, dhcpManager.Stop(state.PIDDNSMasq))
	cleanupErr = errors.Join(cleanupErr, mihomoManager.Stop(state.PIDMihomo))
	if state.PFAnchorLoaded {
		cleanupErr = errors.Join(cleanupErr, pfManager.Unload(!state.PFEnabledBefore))
	}
	cleanupErr = errors.Join(cleanupErr, sysctlManager.Restore(state.IPForwardingBefore))
	cleanupErr = errors.Join(cleanupErr, deps.removeState(m.paths.StateFile))
	cleanupErr = errors.Join(cleanupErr, device.RemovePolicyBundleSnapshot(m.paths.DevicePolicyApplied))
	if cleanupErr != nil {
		return fmt.Errorf("%w; rollback failed: %v", cause, cleanupErr)
	}
	return cause
}
