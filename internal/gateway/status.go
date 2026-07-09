package gateway

import (
	"context"
	"fmt"
	"strings"

	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/dhcp"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/pf"
	"open-mihomo-gateway/internal/runtime"
	"open-mihomo-gateway/internal/sysctl"
)

type Status struct {
	Gateway     string `json:"gateway"`
	Interface   string `json:"interface"`
	LANIP       string `json:"lan_ip"`
	DHCP        string `json:"dhcp"`
	DHCPEnabled bool   `json:"dhcp_enabled"`
	Mihomo      string `json:"mihomo"`
	PFAnchor    string `json:"pf_anchor"`
	Forwarding  string `json:"forwarding"`
	ClientCount int    `json:"client_count"`
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
	if exists {
		dhcpRunning := false
		mihomoRunning := false
		mihomoManager := mihomo.New(m.cfg, m.paths)
		if mihomoManager.Running(state.PIDMihomo) {
			mihomoRunning = true
			mihomoStatus = "running"
			if version, err := mihomo.FetchVersion(ctx, m.cfg); err == nil && version.Version != "" {
				mihomoStatus = "running (" + version.Version + ")"
			}
		}
		dhcpManager := dhcp.New(m.cfg, m.paths)
		if dhcpManager.Running(state.PIDDNSMasq) {
			dhcpRunning = true
			dhcpStatus = "running"
		}
		if dhcpRunning && mihomoRunning {
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
	forwarding := "unknown"
	if current, err := sysctl.New().Current(); err == nil {
		forwarding = sysctl.FormatForwarding(current)
	}

	return Status{
		Gateway:     gatewayStatus,
		Interface:   m.cfg.Gateway.Interface,
		LANIP:       m.cfg.Gateway.LANIP,
		DHCP:        dhcpStatus,
		DHCPEnabled: m.cfg.DHCP.Enabled,
		Mihomo:      mihomoStatus,
		PFAnchor:    pfStatus,
		Forwarding:  forwarding,
		ClientCount: len(clients),
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
		fmt.Sprintf("Clients: %d", s.ClientCount),
	}
	return strings.Join(lines, "\n") + "\n"
}
