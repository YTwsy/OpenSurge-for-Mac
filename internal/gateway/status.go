package gateway

import (
	"context"
	"fmt"
	"strings"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/dhcp"
	"open-mihomo-gateway/internal/macosipv6"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/pf"
	"open-mihomo-gateway/internal/runtime"
	"open-mihomo-gateway/internal/sysctl"
)

type Status struct {
	Gateway             string `json:"gateway"`
	Interface           string `json:"interface"`
	LANIP               string `json:"lan_ip"`
	DHCP                string `json:"dhcp"`
	DHCPEnabled         bool   `json:"dhcp_enabled"`
	Mihomo              string `json:"mihomo"`
	PFAnchor            string `json:"pf_anchor"`
	Forwarding          string `json:"forwarding"`
	DNSIPv6             bool   `json:"dns_ipv6"`
	TUNIPv6Requested    string `json:"tun_ipv6_requested"`
	TUNIPv6Effective    bool   `json:"tun_ipv6_effective"`
	NativeIPv6Available bool   `json:"native_ipv6_available"`
	IPv6Reason          string `json:"ipv6_reason"`
	ClientCount         int    `json:"client_count"`
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
	pfStatus := "unloaded"
	dnsIPv6 := m.cfg.DNS.IPv6
	tunIPv6Requested := m.cfg.Transparent.TUNIPv6
	tunIPv6Effective := false
	nativeIPv6Available := false
	ipv6Reason := "stopped"
	if exists {
		appliedCfg := appliedConfigFromState(m.cfg, state)
		dnsIPv6 = state.DNSIPv6
		tunIPv6Requested = state.TUNIPv6Requested
		nativeIPv6Available = state.NativeIPv6Available
		ipv6Reason = state.IPv6Reason
		if tunIPv6Requested == "" {
			tunIPv6Requested = config.TUNIPv6Off
		}
		if ipv6Reason == "" {
			ipv6Reason = "disabled"
		}
		if state.TUNIPv6Effective {
			tunIPv6Effective = macosipv6.New(appliedCfg).TUNEffective()
			if !tunIPv6Effective {
				ipv6Reason = "tun_ipv6_address_missing"
			}
		}
		dhcpRunning := false
		mihomoRunning := false
		mihomoManager := mihomo.New(appliedCfg, m.paths)
		if mihomoManager.Running(state.PIDMihomo) {
			mihomoRunning = true
			mihomoStatus = "running"
			if version, err := mihomo.FetchVersion(ctx, m.cfg); err == nil && version.Version != "" {
				mihomoStatus = "running (" + version.Version + ")"
			}
		}
		dhcpManager := dhcp.New(appliedCfg, m.paths)
		if dhcpManager.Running(state.PIDDNSMasq) {
			dhcpRunning = true
			dhcpStatus = "running"
		}
		if dhcpRunning && mihomoRunning {
			gatewayStatus = "running"
		} else {
			gatewayStatus = "degraded"
		}
		if gatewayStatus == "running" && state.TUNIPv6Effective && !tunIPv6Effective {
			gatewayStatus = "degraded"
		}
		if state.PFAnchorLoaded {
			pfStatus = "loaded"
			if loaded, err := pf.New(m.cfg, m.paths).Loaded(); err == nil && !loaded {
				pfStatus = "unloaded"
			}
		}
	}
	forwarding := "unknown"
	if current, err := sysctl.New().Current(); err == nil {
		forwarding = sysctl.FormatForwarding(current)
	}
	return Status{
		Gateway:             gatewayStatus,
		Interface:           m.cfg.Gateway.Interface,
		LANIP:               m.cfg.Gateway.LANIP,
		DHCP:                dhcpStatus,
		DHCPEnabled:         m.cfg.DHCP.Enabled,
		Mihomo:              mihomoStatus,
		PFAnchor:            pfStatus,
		Forwarding:          forwarding,
		DNSIPv6:             dnsIPv6,
		TUNIPv6Requested:    tunIPv6Requested,
		TUNIPv6Effective:    tunIPv6Effective,
		NativeIPv6Available: nativeIPv6Available,
		IPv6Reason:          ipv6Reason,
		ClientCount:         len(clients),
	}, nil
}

func (s Status) Format() string {
	dnsmasqLabel := "DHCP"
	if !s.DHCPEnabled {
		dnsmasqLabel = "DNS"
	}
	lines := []string{
		fmt.Sprintf("Gateway: %s", s.Gateway),
		fmt.Sprintf("Interface: %s", s.Interface),
		fmt.Sprintf("LAN IP: %s", s.LANIP),
		fmt.Sprintf("%s: %s", dnsmasqLabel, s.DHCP),
		fmt.Sprintf("mihomo: %s", s.Mihomo),
		fmt.Sprintf("pf anchor: %s", s.PFAnchor),
		fmt.Sprintf("IP forwarding: %s", s.Forwarding),
		fmt.Sprintf("IPv6 DNS queries: %t", s.DNSIPv6),
		fmt.Sprintf("TUN IPv6: requested=%s effective=%t (%s)", s.TUNIPv6Requested, s.TUNIPv6Effective, s.IPv6Reason),
		fmt.Sprintf("Clients: %d", s.ClientCount),
	}
	return strings.Join(lines, "\n") + "\n"
}
