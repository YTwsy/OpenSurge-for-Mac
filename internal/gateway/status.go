package gateway

import (
	"context"
	"fmt"
	"strings"

	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/dhcp"
	"open-mihomo-gateway/internal/runtime"
)

type Status struct {
	Gateway     string
	Interface   string
	LANIP       string
	DHCP        string
	Mihomo      string
	PFAnchor    string
	Forwarding  string
	ClientCount int
}

func (m Manager) Status(_ context.Context) (Status, error) {
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
	if exists {
		dhcpManager := dhcp.New(m.cfg, m.paths)
		if dhcpManager.Running(state.PIDDNSMasq) {
			dhcpStatus = "running"
			gatewayStatus = "running"
		} else {
			gatewayStatus = "degraded"
		}
	}

	return Status{
		Gateway:     gatewayStatus,
		Interface:   m.cfg.Gateway.Interface,
		LANIP:       m.cfg.Gateway.LANIP,
		DHCP:        dhcpStatus,
		Mihomo:      "stopped",
		PFAnchor:    "unloaded",
		Forwarding:  "unknown",
		ClientCount: len(clients),
	}, nil
}

func (s Status) Format() string {
	lines := []string{
		fmt.Sprintf("Gateway: %s", s.Gateway),
		fmt.Sprintf("Interface: %s", s.Interface),
		fmt.Sprintf("LAN IP: %s", s.LANIP),
		fmt.Sprintf("DHCP: %s", s.DHCP),
		fmt.Sprintf("mihomo: %s", s.Mihomo),
		fmt.Sprintf("pf anchor: %s", s.PFAnchor),
		fmt.Sprintf("IP forwarding: %s", s.Forwarding),
		fmt.Sprintf("Clients: %d", s.ClientCount),
	}
	return strings.Join(lines, "\n") + "\n"
}
