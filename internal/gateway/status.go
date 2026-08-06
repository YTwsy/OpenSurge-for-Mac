package gateway

import (
	"context"
	"fmt"
	"strings"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/dhcp"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/pf"
	"open-mihomo-gateway/internal/runtime"
	"open-mihomo-gateway/internal/sysctl"
)

type Status struct {
	Gateway             string `json:"gateway"`
	RuntimeState        string `json:"runtime_state,omitempty"`
	Interface           string `json:"interface"`
	LANIP               string `json:"lan_ip"`
	DHCP                string `json:"dhcp"`
	DHCPEnabled         bool   `json:"dhcp_enabled"`
	Mihomo              string `json:"mihomo"`
	TUN                 string `json:"tun"`
	TUNInterface        string `json:"tun_interface,omitempty"`
	TUNError            string `json:"tun_error,omitempty"`
	PFAnchor            string `json:"pf_anchor"`
	Forwarding          string `json:"forwarding"`
	ClientCount         int    `json:"client_count"`
	DNSIPv6             bool   `json:"dns_ipv6"`
	TUNIPv6Requested    string `json:"tun_ipv6_requested"`
	IPv6Packet          string `json:"ipv6_packet"`
	NativeIPv6Available bool   `json:"native_ipv6_available"`
	IPv6Reason          string `json:"ipv6_reason"`
}

func (m Manager) Status(ctx context.Context) (Status, error) {
	state, exists, err := runtime.LoadState(m.paths.StateFile)
	if err != nil {
		return Status{}, err
	}
	clients, err := device.LoadLeases(m.paths.LeaseFile)
	if err != nil {
		return Status{}, err
	}

	gatewayStatus := "stopped"
	dhcpStatus := "stopped"
	mihomoStatus := "stopped"
	tunStatus := "disabled"
	tunInterface := ""
	tunError := ""
	if m.cfg.Transparent.TUNEnabled() {
		tunStatus = "stopped"
	}
	pfStatus := "unloaded"
	runtimeState := "none"
	dnsIPv6 := m.cfg.DNS.IPv6
	tunIPv6Requested := m.cfg.Transparent.TUNIPv6
	ipv6PacketStatus := "disabled"
	nativeIPv6Available := false
	ipv6Reason := "stopped"
	if m.cfg.Transparent.TUNIPv6 != config.TUNIPv6Off {
		ipv6PacketStatus = "stopped"
	}
	if exists {
		dnsIPv6 = state.DNSIPv6
		tunIPv6Requested = state.TUNIPv6Requested
		nativeIPv6Available = state.NativeIPv6Available
		ipv6Reason = state.IPv6Reason
		bootSession, bootErr := runtime.CurrentBootSession()
		if bootErr != nil {
			return Status{}, fmt.Errorf("determine current boot session: %w", bootErr)
		}
		if !state.BelongsToBoot(bootSession) {
			gatewayStatus = "degraded"
			runtimeState = "interrupted"
		} else {
			runtimeState = "active"
			appliedCfg := appliedConfigFromState(m.cfg, state)
			dhcpRunning := false
			mihomoRunning := false
			ipv6PacketRunning := !state.IPv6PacketEffective
			mihomoManager := mihomo.New(appliedCfg, m.paths)
			if trackedProcessRunning(m.gatewayDeps(), state.PIDMihomo, state.MihomoProcessFingerprint, mihomoManager.Running) {
				mihomoRunning = true
				mihomoStatus = "running"
				version, versionErr, runtimeTUN, tunErr := fetchMihomoRuntime(ctx, appliedCfg)
				if versionErr == nil && version.Version != "" {
					mihomoStatus = "running (" + version.Version + ")"
				}
				if m.cfg.Transparent.TUNEnabled() {
					switch {
					case tunErr != nil:
						tunStatus = "unknown"
						tunError = tunErr.Error()
					case runtimeTUN.Enabled:
						tunStatus = "ready"
						tunInterface = runtimeTUN.Device
					default:
						tunStatus = "failed"
						tunInterface = runtimeTUN.Device
						tunError = "mihomo runtime config reports TUN disabled"
					}
				}
			}
			if state.IPv6PacketEffective {
				packetManager := Manager{cfg: appliedCfg, paths: m.paths, deps: m.deps}.ipv6Packet(m.gatewayDeps())
				if trackedProcessRunning(m.gatewayDeps(), state.PIDIPv6Packet, state.IPv6PacketFingerprint, packetManager.Running) {
					ipv6PacketRunning = true
					ipv6PacketStatus = "ready"
				} else {
					ipv6PacketStatus = "failed"
				}
			}
			dhcpManager := dhcp.New(m.cfg, m.paths)
			if trackedProcessRunning(m.gatewayDeps(), state.PIDDNSMasq, state.DNSMasqProcessFingerprint, dhcpManager.Running) {
				dhcpRunning = true
				dhcpStatus = "running"
			}
			// A failed runtime read is an observability warning, not evidence that
			// the already-running TUN data plane stopped. An explicit disabled
			// response remains a real degraded condition.
			tunReady := !m.cfg.Transparent.TUNEnabled() || tunStatus == "ready" || tunStatus == "unknown"
			if dhcpRunning && mihomoRunning && tunReady && ipv6PacketRunning {
				gatewayStatus = "running"
			} else {
				gatewayStatus = "degraded"
			}
			if state.PFAnchorLoaded {
				pfStatus = "loaded"
				if loaded, err := pf.New(m.cfg, m.paths).Loaded(); err == nil && !loaded {
					pfStatus = "unloaded"
				}
			}
		}
	}
	forwarding := "unknown"
	if current, err := sysctl.New().Current(); err == nil {
		forwarding = sysctl.FormatForwarding(current)
	}

	return Status{
		Gateway:             gatewayStatus,
		RuntimeState:        runtimeState,
		Interface:           m.cfg.Gateway.Interface,
		LANIP:               m.cfg.Gateway.LANIP,
		DHCP:                dhcpStatus,
		DHCPEnabled:         m.cfg.DHCP.Enabled,
		Mihomo:              mihomoStatus,
		TUN:                 tunStatus,
		TUNInterface:        tunInterface,
		TUNError:            tunError,
		PFAnchor:            pfStatus,
		Forwarding:          forwarding,
		ClientCount:         len(clients),
		DNSIPv6:             dnsIPv6,
		TUNIPv6Requested:    tunIPv6Requested,
		IPv6Packet:          ipv6PacketStatus,
		NativeIPv6Available: nativeIPv6Available,
		IPv6Reason:          ipv6Reason,
	}, nil
}

type versionResult struct {
	value mihomo.Version
	err   error
}

type tunResult struct {
	value mihomo.TUNRuntimeState
	err   error
}

func fetchMihomoRuntime(ctx context.Context, cfg config.Config) (mihomo.Version, error, mihomo.TUNRuntimeState, error) {
	if !cfg.Transparent.TUNEnabled() {
		version, err := mihomo.FetchVersion(ctx, cfg)
		return version, err, mihomo.TUNRuntimeState{}, nil
	}
	versionCh := make(chan versionResult, 1)
	tunCh := make(chan tunResult, 1)
	go func() {
		value, err := mihomo.FetchVersion(ctx, cfg)
		versionCh <- versionResult{value: value, err: err}
	}()
	go func() {
		value, err := mihomo.FetchTUNRuntimeState(ctx, cfg)
		tunCh <- tunResult{value: value, err: err}
	}()
	version := <-versionCh
	tun := <-tunCh
	return version.value, version.err, tun.value, tun.err
}

func (s Status) Format() string {
	dnsmasqLabel := "DHCP"
	if !s.DHCPEnabled {
		dnsmasqLabel = "DNS"
	}
	tunLabel := s.TUN
	if s.TUNInterface != "" {
		tunLabel += " (" + s.TUNInterface + ")"
	}
	if s.TUNError != "" {
		tunLabel += ": " + s.TUNError
	}
	lines := []string{
		fmt.Sprintf("Gateway: %s", s.Gateway),
		fmt.Sprintf("Runtime state: %s", s.RuntimeState),
		fmt.Sprintf("Interface: %s", s.Interface),
		fmt.Sprintf("LAN IP: %s", s.LANIP),
		fmt.Sprintf("%s: %s", dnsmasqLabel, s.DHCP),
		fmt.Sprintf("mihomo: %s", s.Mihomo),
		fmt.Sprintf("TUN: %s", tunLabel),
		fmt.Sprintf("IPv6 DNS queries: %t", s.DNSIPv6),
		fmt.Sprintf("IPv6 packet path: requested=%s state=%s (%s)", s.TUNIPv6Requested, s.IPv6Packet, s.IPv6Reason),
		fmt.Sprintf("pf anchor: %s", s.PFAnchor),
		fmt.Sprintf("IP forwarding: %s", s.Forwarding),
		fmt.Sprintf("Clients: %d", s.ClientCount),
	}
	return strings.Join(lines, "\n") + "\n"
}
