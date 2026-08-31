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
	"open-mihomo-gateway/internal/ipv6packet"
	"open-mihomo-gateway/internal/macosipv6"
	"open-mihomo-gateway/internal/macosnetwork"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/pf"
	"open-mihomo-gateway/internal/process"
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

type localSystemProxyService interface {
	Prepare(context.Context, string, int) (runtime.SystemProxySnapshot, error)
	Enable(context.Context, runtime.SystemProxySnapshot, int) error
	Restore(context.Context, runtime.SystemProxySnapshot) error
}

type ipv6PacketService interface {
	Check() error
	Start() (int, error)
	Stop(int) error
	Running(int) bool
}

type ipv6HostService interface {
	NativeAvailable() (bool, error)
	CheckGatewayAvailable() error
	AddGateway(context.Context) error
	RemoveGateway(context.Context) error
	Withdraw(context.Context) error
}

type gatewayDeps struct {
	geteuid             func() int
	loadState           func(string) (runtime.State, bool, error)
	saveState           func(string, runtime.State) error
	removeState         func(string) error
	ensure              func(runtime.Paths) error
	newDHCP             func(config.Config, runtime.Paths) dhcpService
	newMihomo           func(config.Config, runtime.Paths) mihomoService
	newPF               func(config.Config, runtime.Paths) pfService
	newSysctl           func() sysctlService
	newLocalSystemProxy func() localSystemProxyService
	newIPv6Packet       func(config.Config, runtime.Paths) ipv6PacketService
	newIPv6Host         func(config.Config) ipv6HostService
	interfaces          func() ([]net.Interface, error)
	interfaceByName     func(string) (*net.Interface, error)
	interfaceAddrs      func(*net.Interface) ([]net.Addr, error)
	probeReservationIP  func(ip string, expectedMAC string) error
	currentBoot         func() (runtime.BootSession, error)
	processFingerprint  func(int) (string, error)
	processMatches      func(int, string) (bool, error)
	warmTailscale       func(context.Context, config.Config) mihomo.ProxyDelayResult
	now                 func() time.Time
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
		newLocalSystemProxy: func() localSystemProxyService {
			return macosnetwork.SystemProxy{}
		},
		newIPv6Packet: func(cfg config.Config, paths runtime.Paths) ipv6PacketService {
			manager := ipv6packet.NewManager(cfg, paths)
			return manager
		},
		newIPv6Host: func(cfg config.Config) ipv6HostService {
			manager := macosipv6.New(cfg)
			return manager
		},
		interfaces:      net.Interfaces,
		interfaceByName: net.InterfaceByName,
		interfaceAddrs: func(iface *net.Interface) ([]net.Addr, error) {
			return iface.Addrs()
		},
		probeReservationIP: probeReservationIPConflict,
		currentBoot:        runtime.CurrentBootSession,
		processFingerprint: process.Fingerprint,
		processMatches:     process.MatchesFingerprint,
		warmTailscale:      mihomo.WarmTailscale,
		now:                time.Now,
	}
}

func (m Manager) ipv6Packet(deps gatewayDeps) ipv6PacketService {
	if deps.newIPv6Packet != nil {
		return deps.newIPv6Packet(m.cfg, m.paths)
	}
	manager := ipv6packet.NewManager(m.cfg, m.paths)
	return manager
}

func (m Manager) ipv6Host(deps gatewayDeps) ipv6HostService {
	if deps.newIPv6Host != nil {
		return deps.newIPv6Host(m.cfg)
	}
	manager := macosipv6.New(m.cfg)
	return manager
}

type ipv6Resolution struct {
	Requested       string
	NativeAvailable bool
	Effective       bool
	Reason          string
}

func (m *Manager) resolveIPv6(deps gatewayDeps) ipv6Resolution {
	resolution := ipv6Resolution{Requested: m.cfg.Transparent.TUNIPv6, Reason: "disabled"}
	switch m.cfg.Transparent.TUNIPv6 {
	case config.TUNIPv6Always:
		resolution.Effective = true
		resolution.Reason = "forced_userspace_packet_path"
		available, err := m.ipv6Host(deps).NativeAvailable()
		if err != nil {
			// always is an explicit force-on choice: upstream detection remains
			// observability and must not turn it back off.
			resolution.Reason += "; native_detection_failed: " + err.Error()
		} else {
			resolution.NativeAvailable = available
		}
	case config.TUNIPv6Auto:
		available, err := m.ipv6Host(deps).NativeAvailable()
		if err != nil {
			m.cfg.Transparent.TUNIPv6 = config.TUNIPv6Off
			resolution.Reason = "native_detection_failed: " + err.Error()
			return resolution
		}
		resolution.NativeAvailable = available
		resolution.Effective = available
		if available {
			resolution.Reason = "native_ipv6_available"
		} else {
			m.cfg.Transparent.TUNIPv6 = config.TUNIPv6Off
			resolution.Reason = "native_ipv6_unavailable"
		}
	}
	return resolution
}

func appliedConfigFromState(cfg config.Config, state runtime.State) config.Config {
	cfg.DNS.IPv6 = state.DNSIPv6
	if state.IPv6PacketEffective {
		cfg.Transparent.TUNIPv6 = state.TUNIPv6Requested
	} else {
		cfg.Transparent.TUNIPv6 = config.TUNIPv6Off
	}
	return cfg
}

func (m Manager) gatewayDeps() gatewayDeps {
	if m.deps.geteuid == nil {
		return defaultGatewayDeps()
	}
	return m.deps
}

func (m Manager) localSystemProxy(deps gatewayDeps) localSystemProxyService {
	if deps.newLocalSystemProxy != nil {
		return deps.newLocalSystemProxy()
	}
	return macosnetwork.SystemProxy{}
}

func currentBoot(deps gatewayDeps) (runtime.BootSession, error) {
	if deps.currentBoot != nil {
		return deps.currentBoot()
	}
	return runtime.CurrentBootSession()
}

func processFingerprint(deps gatewayDeps, pid int) (string, error) {
	if deps.processFingerprint != nil {
		return deps.processFingerprint(pid)
	}
	return process.Fingerprint(pid)
}

func processMatches(deps gatewayDeps, pid int, fingerprint string) (bool, error) {
	if deps.processMatches != nil {
		return deps.processMatches(pid, fingerprint)
	}
	return process.MatchesFingerprint(pid, fingerprint)
}

func stopTrackedProcess(deps gatewayDeps, name string, pid int, fingerprint string, stop func(int) error) error {
	if strings.TrimSpace(fingerprint) == "" {
		return stop(pid)
	}
	matches, err := processMatches(deps, pid, fingerprint)
	if err != nil {
		return fmt.Errorf("verify %s pid %d before stop: %w", name, pid, err)
	}
	if !matches {
		return nil
	}
	return stop(pid)
}

func trackedProcessRunning(deps gatewayDeps, pid int, fingerprint string, running func(int) bool) bool {
	if strings.TrimSpace(fingerprint) == "" {
		return running(pid)
	}
	matches, err := processMatches(deps, pid, fingerprint)
	return err == nil && matches && running(pid)
}

func (m Manager) Start(ctx context.Context) error {
	if m.gatewayDeps().geteuid() != 0 {
		return fmt.Errorf("start requires sudo/root privileges")
	}
	return m.withLifecycleLock(func() error { return m.start(ctx) })
}

func (m Manager) start(ctx context.Context) error {
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
	ipv6Resolution := m.resolveIPv6(deps)
	if err := deps.ensure(m.paths); err != nil {
		return err
	}
	bootSession, err := currentBoot(deps)
	if err != nil {
		return fmt.Errorf("determine current boot session: %w", err)
	}

	dhcpManager := deps.newDHCP(m.cfg, m.paths)
	mihomoManager := deps.newMihomo(m.cfg, m.paths)
	pfManager := deps.newPF(m.cfg, m.paths)
	sysctlManager := deps.newSysctl()
	systemProxyManager := m.localSystemProxy(deps)
	ipv6PacketManager := m.ipv6Packet(deps)
	ipv6HostManager := m.ipv6Host(deps)
	if ipv6Resolution.Effective {
		if err := ipv6HostManager.CheckGatewayAvailable(); err != nil {
			return err
		}
	}
	if err := m.preflight(dhcpManager, mihomoManager, pfManager, sysctlManager, deps); err != nil {
		return err
	}
	if err := m.checkReservationConflicts(deps); err != nil {
		return err
	}
	var systemProxySnapshot *runtime.SystemProxySnapshot
	if m.cfg.LocalSystemProxy.Enabled {
		snapshot, err := systemProxyManager.Prepare(ctx, m.cfg.Gateway.UpstreamInterface, m.cfg.Mihomo.MixedPort)
		if err != nil {
			return fmt.Errorf("prepare local system proxy coordination: %w", err)
		}
		systemProxySnapshot = &snapshot
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
	ipForwardingBefore, err := sysctlManager.Current()
	if err != nil {
		return err
	}
	pfEnabledBefore, err := pfManager.Enabled()
	if err != nil {
		return err
	}
	profileDigest, err := config.MihomoProfileDigest(m.cfg)
	if err != nil {
		return fmt.Errorf("digest imported mihomo profile: %w", err)
	}
	if bundle := m.cfg.DevicePolicy.Bundle; bundle != nil {
		if err := dhcp.ReconcilePolicyLeases(m.paths.LeaseFile, bundle.Compiled.Reservations); err != nil {
			return err
		}
		if err := device.WritePolicyBundleSnapshot(m.paths.DevicePolicyApplied, *bundle); err != nil {
			return err
		}
	}

	state := runtime.State{
		StartedAt:           deps.now(),
		BootSessionID:       bootSession.ID,
		IPForwardingBefore:  ipForwardingBefore,
		PFEnabledBefore:     pfEnabledBefore,
		ProfileDigest:       profileDigest,
		LocalSystemProxy:    systemProxySnapshot,
		DNSIPv6:             m.cfg.DNS.IPv6,
		TUNIPv6Requested:    ipv6Resolution.Requested,
		IPv6PacketEffective: ipv6Resolution.Effective,
		NativeIPv6Available: ipv6Resolution.NativeAvailable,
		IPv6Reason:          ipv6Resolution.Reason,
		IPv6RAEffective:     ipv6Resolution.Effective && m.cfg.DHCP.Enabled,
	}
	if bundle := m.cfg.DevicePolicy.Bundle; bundle != nil {
		state.DevicePolicyDigest = bundle.Digest
	}
	if err := deps.saveState(m.paths.StateFile, state); err != nil {
		_ = device.RemovePolicyBundleSnapshot(m.paths.DevicePolicyApplied)
		return err
	}

	if err := sysctlManager.Enable(); err != nil {
		return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
	}

	mihomoPID, err := mihomoManager.Start()
	if err != nil {
		return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
	}
	state.PIDMihomo = mihomoPID
	state.MihomoProcessFingerprint, err = processFingerprint(deps, mihomoPID)
	if err != nil || (mihomoPID > 0 && state.MihomoProcessFingerprint == "") {
		if err == nil {
			err = fmt.Errorf("mihomo pid %d disappeared before its identity could be recorded", mihomoPID)
		}
		return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
	}
	if err := deps.saveState(m.paths.StateFile, state); err != nil {
		return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
	}
	if ipv6Resolution.Effective {
		ipv6PacketPID, startErr := ipv6PacketManager.Start()
		if startErr != nil {
			return m.rollback(ctx, startErr, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
		}
		state.PIDIPv6Packet = ipv6PacketPID
		state.IPv6PacketFingerprint, err = processFingerprint(deps, ipv6PacketPID)
		if err != nil || (ipv6PacketPID > 0 && state.IPv6PacketFingerprint == "") {
			if err == nil {
				err = fmt.Errorf("IPv6 packet broker pid %d disappeared before its identity could be recorded", ipv6PacketPID)
			}
			return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
		}
		// Persist broker identity before changing the host interface. If the
		// following alias step fails, a failed rollback remains retryable and
		// never leaves an untracked root packet process behind.
		if err := deps.saveState(m.paths.StateFile, state); err != nil {
			return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
		}
		// Persist alias ownership intent before mutating the interface. Removal is
		// idempotent when the add command never took effect, while a successful add
		// can no longer fall into an untracked gap if DAD or the next state write
		// fails.
		state.IPv6GatewayAliasOwned = true
		if err := deps.saveState(m.paths.StateFile, state); err != nil {
			return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
		}
		if err := ipv6HostManager.AddGateway(ctx); err != nil {
			return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
		}
	}

	pid, err := dhcpManager.Start()
	if err != nil {
		return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
	}
	state.PIDDNSMasq = pid
	state.DNSMasqProcessFingerprint, err = processFingerprint(deps, pid)
	if err != nil || (pid > 0 && state.DNSMasqProcessFingerprint == "") {
		if err == nil {
			err = fmt.Errorf("dnsmasq pid %d disappeared before its identity could be recorded", pid)
		}
		return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
	}
	if err := deps.saveState(m.paths.StateFile, state); err != nil {
		return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
	}

	if err := pfManager.Load(!pfEnabledBefore); err != nil {
		return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
	}
	state.PFAnchorLoaded = true
	loaded, err := pfManager.Loaded()
	if err != nil {
		return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
	}
	if !loaded {
		return m.rollback(ctx, fmt.Errorf("pf anchor %s did not become visible after load", m.cfg.PF.AnchorName), state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
	}
	if err := deps.saveState(m.paths.StateFile, state); err != nil {
		return m.rollback(ctx, err, state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, false)
	}
	if state.LocalSystemProxy != nil {
		if err := systemProxyManager.Enable(ctx, *state.LocalSystemProxy, m.cfg.Mihomo.MixedPort); err != nil {
			return m.rollback(ctx, fmt.Errorf("enable local system proxy coordination: %w", err), state, dhcpManager, mihomoManager, pfManager, sysctlManager, systemProxyManager, true)
		}
	}
	m.warmManagedTailscale(ctx, deps)

	fmt.Printf("Gateway runtime prepared in %s\n", m.paths.Dir)
	if mihomoPID > 0 {
		fmt.Printf("mihomo started with pid %d\n", mihomoPID)
	}
	if pid > 0 {
		fmt.Printf("dnsmasq started with pid %d\n", pid)
	}
	fmt.Printf("pf anchor %s loaded\n", m.cfg.PF.AnchorName)
	if state.LocalSystemProxy != nil {
		fmt.Printf("macOS HTTP/HTTPS system proxy enabled for network service %s\n", state.LocalSystemProxy.NetworkService)
	}
	return nil
}

// Reload validates a complete desired candidate before touching the running
// gateway, then performs the same audited stop/start lifecycle as the normal
// commands. The Manager owns an immutable Config value, so the configuration
// that passed validation is also the configuration applied after stop.
func (m Manager) Reload(ctx context.Context) error {
	if m.gatewayDeps().geteuid() != 0 {
		return fmt.Errorf("reload requires sudo/root privileges")
	}
	return m.withLifecycleLock(func() error { return m.reload(ctx) })
}

func (m Manager) reload(ctx context.Context) error {
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
	bootSession, err := currentBoot(deps)
	if err != nil {
		return fmt.Errorf("determine current boot session: %w", err)
	}
	if !state.BelongsToBoot(bootSession) {
		return fmt.Errorf("gateway runtime was interrupted by a system reboot; run stop to clean the interrupted runtime, then start the gateway")
	}
	dhcpManager := deps.newDHCP(m.cfg, m.paths)
	appliedCfg := appliedConfigFromState(m.cfg, state)
	mihomoManager := deps.newMihomo(appliedCfg, m.paths)
	if !trackedProcessRunning(deps, state.PIDDNSMasq, state.DNSMasqProcessFingerprint, dhcpManager.Running) || !trackedProcessRunning(deps, state.PIDMihomo, state.MihomoProcessFingerprint, mihomoManager.Running) {
		return fmt.Errorf("gateway is degraded; reload requires both DHCP/DNS and mihomo to be running")
	}
	if state.IPv6PacketEffective {
		packetManager := Manager{cfg: appliedCfg, paths: m.paths, deps: m.deps}.ipv6Packet(deps)
		if !trackedProcessRunning(deps, state.PIDIPv6Packet, state.IPv6PacketFingerprint, packetManager.Running) {
			return fmt.Errorf("gateway is degraded; reload requires the IPv6 packet broker to be running")
		}
	}
	if err := m.validateReloadCandidate(); err != nil {
		return fmt.Errorf("reload candidate validation failed: %w", err)
	}
	if err := m.stop(ctx); err != nil {
		return fmt.Errorf("reload stop failed: %w", err)
	}
	if err := m.start(ctx); err != nil {
		return fmt.Errorf("reload start failed after gateway stop: %w", err)
	}
	return nil
}

// RestartMihomo rebuilds only the proxy engine process. It deliberately keeps
// dnsmasq, PF, IPv4 forwarding, and the host network configuration untouched,
// so an upstream link recovery does not turn into a full gateway takeover
// transition. The existing rendered configuration is validated before the
// live process is stopped, and the previous log is archived for diagnosis.
func (m Manager) RestartMihomo(ctx context.Context) error {
	if m.gatewayDeps().geteuid() != 0 {
		return fmt.Errorf("restart-mihomo requires sudo/root privileges")
	}
	return m.withLifecycleLock(func() error { return m.restartMihomo(ctx) })
}

func (m Manager) restartMihomo(ctx context.Context) error {
	deps := m.gatewayDeps()
	if deps.geteuid() != 0 {
		return fmt.Errorf("restart-mihomo requires sudo/root privileges")
	}
	state, exists, err := deps.loadState(m.paths.StateFile)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("gateway is not running; run start instead")
	}
	bootSession, err := currentBoot(deps)
	if err != nil {
		return fmt.Errorf("determine current boot session: %w", err)
	}
	if !state.BelongsToBoot(bootSession) {
		return fmt.Errorf("gateway runtime was interrupted by a system reboot; run stop to clean the interrupted runtime, then start the gateway")
	}
	desiredProfileDigest, err := config.MihomoProfileDigest(m.cfg)
	if err != nil {
		return fmt.Errorf("digest current imported mihomo profile: %w", err)
	}
	if desiredProfileDigest != state.ProfileDigest {
		return fmt.Errorf("desired imported mihomo profile differs from the applied runtime; run reload instead")
	}

	appliedCfg := appliedConfigFromState(m.cfg, state)
	mihomoManager := deps.newMihomo(appliedCfg, m.paths)
	systemProxyManager := m.localSystemProxy(deps)
	restoreSystemProxy := func() error {
		if state.LocalSystemProxy == nil {
			return nil
		}
		return systemProxyManager.Restore(ctx, *state.LocalSystemProxy)
	}
	if err := mihomoManager.ValidateWrittenConfig(); err != nil {
		return fmt.Errorf("prepared mihomo config validation failed: %w", err)
	}

	previousPID := state.PIDMihomo
	previousFingerprint := state.MihomoProcessFingerprint
	state.PIDMihomo = 0
	state.MihomoProcessFingerprint = ""
	if err := deps.saveState(m.paths.StateFile, state); err != nil {
		return fmt.Errorf("mark mihomo restart in runtime state: %w", err)
	}
	if err := stopTrackedProcess(deps, "mihomo", previousPID, previousFingerprint, mihomoManager.Stop); err != nil {
		if trackedProcessRunning(deps, previousPID, previousFingerprint, mihomoManager.Running) {
			state.PIDMihomo = previousPID
			state.MihomoProcessFingerprint = previousFingerprint
			return errors.Join(fmt.Errorf("stop mihomo pid %d: %w", previousPID, err), deps.saveState(m.paths.StateFile, state))
		}
		return errors.Join(fmt.Errorf("stop mihomo pid %d: %w", previousPID, err), restoreSystemProxy(), deps.saveState(m.paths.StateFile, state))
	}

	archivedLog, err := archiveMihomoLog(m.paths.MihomoLog, deps.now())
	if err != nil {
		return errors.Join(fmt.Errorf("archive mihomo log before restart: %w", err), restoreSystemProxy())
	}
	newPID, err := mihomoManager.Start()
	if err != nil {
		return errors.Join(fmt.Errorf("start replacement mihomo process: %w", err), restoreSystemProxy())
	}
	state.PIDMihomo = newPID
	state.MihomoProcessFingerprint, err = processFingerprint(deps, newPID)
	if err != nil || (newPID > 0 && state.MihomoProcessFingerprint == "") {
		if err == nil {
			err = fmt.Errorf("replacement mihomo pid %d disappeared before its identity could be recorded", newPID)
		}
		restoreErr := restoreSystemProxy()
		stopErr := mihomoManager.Stop(newPID)
		return errors.Join(err, restoreErr, stopErr)
	}
	if err := deps.saveState(m.paths.StateFile, state); err != nil {
		restoreErr := restoreSystemProxy()
		stopErr := mihomoManager.Stop(newPID)
		return errors.Join(fmt.Errorf("save replacement mihomo pid: %w", err), restoreErr, stopErr)
	}
	m.warmManagedTailscale(ctx, deps)

	fmt.Printf("mihomo restarted with pid %d\n", newPID)
	if archivedLog != "" {
		fmt.Printf("previous mihomo log archived at %s\n", archivedLog)
	}
	return nil
}

func (m Manager) warmManagedTailscale(ctx context.Context, deps gatewayDeps) {
	if !m.cfg.Tailscale.Enabled || deps.warmTailscale == nil {
		return
	}
	warmCtx, cancel := context.WithTimeout(ctx, managedTailscaleWarmupTimeout(m.cfg))
	defer cancel()
	result := deps.warmTailscale(warmCtx, m.cfg)
	if result.Status == "reachable" {
		fmt.Printf("Tailscale outbound warm-up completed in %d ms\n", result.DelayMS)
		return
	}
	fmt.Printf("Tailscale outbound warm-up initiated; the first Tailnet request may still need a retry: %s\n", result.Status)
}

func managedTailscaleWarmupTimeout(cfg config.Config) time.Duration {
	if cfg.Tailscale.ExitNode != "" {
		// Leave the HTTP client enough time to report its own role-specific
		// result instead of canceling the outer gateway warm-up first.
		return mihomo.DefaultTailscaleExitNodeTestTimeout + 2*time.Second
	}
	return 6 * time.Second
}

func archiveMihomoLog(path string, now time.Time) (string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	archive := filepath.Join(filepath.Dir(path), fmt.Sprintf("%s-before-restart-%s%s", base, now.UTC().Format("20060102T150405.000000000Z"), ext))
	if err := os.Rename(path, archive); err != nil {
		return "", err
	}
	return archive, nil
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
	candidate.resolveIPv6(deps)
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

func (m Manager) Stop(ctx context.Context) error {
	if m.gatewayDeps().geteuid() != 0 {
		return fmt.Errorf("stop requires sudo/root privileges")
	}
	return m.withLifecycleLock(func() error { return m.stop(ctx) })
}

func (m Manager) stop(ctx context.Context) error {
	deps := m.gatewayDeps()
	if deps.geteuid() != 0 {
		return fmt.Errorf("stop requires sudo/root privileges")
	}
	state, exists, err := deps.loadState(m.paths.StateFile)
	if err != nil {
		return err
	}
	if exists {
		bootSession, bootErr := currentBoot(deps)
		if bootErr != nil {
			return fmt.Errorf("determine current boot session: %w", bootErr)
		}
		if !state.BelongsToBoot(bootSession) {
			return m.cleanupInterruptedRuntime(ctx, deps, state)
		}
	}
	var cleanupErr error
	pfManager := deps.newPF(m.cfg, m.paths)
	sysctlManager := deps.newSysctl()
	if exists {
		if state.LocalSystemProxy != nil {
			if err := m.localSystemProxy(deps).Restore(ctx, *state.LocalSystemProxy); err != nil {
				return fmt.Errorf("restore local system proxy before stopping gateway services: %w", err)
			}
		}
		dhcpManager := deps.newDHCP(m.cfg, m.paths)
		cleanupErr = errors.Join(cleanupErr, stopTrackedProcess(deps, "dnsmasq", state.PIDDNSMasq, state.DNSMasqProcessFingerprint, dhcpManager.Stop))
		cleanupErr = errors.Join(cleanupErr, m.cleanupIPv6(ctx, deps, state))
		mihomoManager := deps.newMihomo(appliedConfigFromState(m.cfg, state), m.paths)
		cleanupErr = errors.Join(cleanupErr, stopTrackedProcess(deps, "mihomo", state.PIDMihomo, state.MihomoProcessFingerprint, mihomoManager.Stop))
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

func (m Manager) cleanupIPv6(ctx context.Context, deps gatewayDeps, state runtime.State) error {
	if !state.IPv6PacketEffective && state.PIDIPv6Packet == 0 && !state.IPv6GatewayAliasOwned {
		return nil
	}
	applied := m
	applied.cfg = appliedConfigFromState(m.cfg, state)
	var cleanupErr error
	if state.IPv6GatewayAliasOwned {
		host := applied.ipv6Host(deps)
		// IPv6RAEffective was added after the first isolated-LAN implementation.
		// Fall back to the applied DHCP-owning topology so an older runtime state
		// still receives a withdrawal during an upgrade stop.
		if state.IPv6RAEffective || (state.IPv6PacketEffective && applied.cfg.DHCP.Enabled) {
			cleanupErr = errors.Join(cleanupErr, host.Withdraw(ctx))
		}
		cleanupErr = errors.Join(cleanupErr, host.RemoveGateway(ctx))
	}
	if state.PIDIPv6Packet != 0 {
		packet := applied.ipv6Packet(deps)
		cleanupErr = errors.Join(cleanupErr, stopTrackedProcess(deps, "IPv6 packet broker", state.PIDIPv6Packet, state.IPv6PacketFingerprint, packet.Stop))
	}
	return cleanupErr
}

func (m Manager) cleanupInterruptedRuntime(ctx context.Context, deps gatewayDeps, state runtime.State) error {
	if state.LocalSystemProxy != nil {
		if err := m.localSystemProxy(deps).Restore(ctx, *state.LocalSystemProxy); err != nil {
			return fmt.Errorf("restore local system proxy snapshot after reboot: %w", err)
		}
	}
	cleanupErr := errors.Join(
		deps.removeState(m.paths.StateFile),
		device.RemovePolicyBundleSnapshot(m.paths.DevicePolicyApplied),
	)
	if cleanupErr != nil {
		return cleanupErr
	}
	fmt.Println("Interrupted gateway runtime from a previous system boot was cleared without signaling stale PIDs or changing current PF/forwarding state.")
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
	if m.cfg.Transparent.TUNIPv6 != config.TUNIPv6Off {
		if err := m.ipv6Packet(deps).Check(); err != nil {
			return fmt.Errorf("IPv6 packet broker: %w", err)
		}
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

func (m Manager) rollback(ctx context.Context, cause error, state runtime.State, dhcpManager dhcpService, mihomoManager mihomoService, pfManager pfService, sysctlManager sysctlService, systemProxyManager localSystemProxyService, restoreSystemProxy bool) error {
	deps := m.gatewayDeps()
	var cleanupErr error
	if restoreSystemProxy && state.LocalSystemProxy != nil {
		if err := systemProxyManager.Restore(ctx, *state.LocalSystemProxy); err != nil {
			return fmt.Errorf("%w; rollback could not restore the local system proxy, so gateway services were left running for recovery: %v", cause, err)
		}
	}
	cleanupErr = errors.Join(cleanupErr, stopTrackedProcess(deps, "dnsmasq", state.PIDDNSMasq, state.DNSMasqProcessFingerprint, dhcpManager.Stop))
	cleanupErr = errors.Join(cleanupErr, m.cleanupIPv6(ctx, deps, state))
	cleanupErr = errors.Join(cleanupErr, stopTrackedProcess(deps, "mihomo", state.PIDMihomo, state.MihomoProcessFingerprint, mihomoManager.Stop))
	if state.PFAnchorLoaded {
		cleanupErr = errors.Join(cleanupErr, pfManager.Unload(!state.PFEnabledBefore))
	}
	cleanupErr = errors.Join(cleanupErr, sysctlManager.Restore(state.IPForwardingBefore))
	if cleanupErr != nil {
		return fmt.Errorf("%w; rollback failed and runtime state was retained for recovery: %v", cause, cleanupErr)
	}
	cleanupErr = errors.Join(cleanupErr, deps.removeState(m.paths.StateFile))
	cleanupErr = errors.Join(cleanupErr, device.RemovePolicyBundleSnapshot(m.paths.DevicePolicyApplied))
	if cleanupErr != nil {
		return fmt.Errorf("%w; rollback cleanup failed: %v", cause, cleanupErr)
	}
	return cause
}
