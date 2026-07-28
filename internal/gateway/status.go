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
	Gateway      string `json:"gateway"`
	Interface    string `json:"interface"`
	LANIP        string `json:"lan_ip"`
	DHCP         string `json:"dhcp"`
	DHCPEnabled  bool   `json:"dhcp_enabled"`
	Mihomo       string `json:"mihomo"`
	TUN          string `json:"tun"`
	TUNInterface string `json:"tun_interface,omitempty"`
	TUNError     string `json:"tun_error,omitempty"`
	PFAnchor     string `json:"pf_anchor"`
	Forwarding   string `json:"forwarding"`
	ClientCount  int    `json:"client_count"`
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
	if exists {
		dhcpRunning := false
		mihomoRunning := false
		mihomoManager := mihomo.New(m.cfg, m.paths)
		if mihomoManager.Running(state.PIDMihomo) {
			mihomoRunning = true
			mihomoStatus = "running"
			version, versionErr, runtimeTUN, tunErr := fetchMihomoRuntime(ctx, m.cfg)
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
		dhcpManager := dhcp.New(m.cfg, m.paths)
		if dhcpManager.Running(state.PIDDNSMasq) {
			dhcpRunning = true
			dhcpStatus = "running"
		}
		tunReady := !m.cfg.Transparent.TUNEnabled() || tunStatus == "ready"
		if dhcpRunning && mihomoRunning && tunReady {
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
		Gateway:      gatewayStatus,
		Interface:    m.cfg.Gateway.Interface,
		LANIP:        m.cfg.Gateway.LANIP,
		DHCP:         dhcpStatus,
		DHCPEnabled:  m.cfg.DHCP.Enabled,
		Mihomo:       mihomoStatus,
		TUN:          tunStatus,
		TUNInterface: tunInterface,
		TUNError:     tunError,
		PFAnchor:     pfStatus,
		Forwarding:   forwarding,
		ClientCount:  len(clients),
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
		fmt.Sprintf("Interface: %s", s.Interface),
		fmt.Sprintf("LAN IP: %s", s.LANIP),
		fmt.Sprintf("%s: %s", dnsmasqLabel, s.DHCP),
		fmt.Sprintf("mihomo: %s", s.Mihomo),
		fmt.Sprintf("TUN: %s", tunLabel),
		fmt.Sprintf("pf anchor: %s", s.PFAnchor),
		fmt.Sprintf("IP forwarding: %s", s.Forwarding),
		fmt.Sprintf("Clients: %d", s.ClientCount),
	}
	return strings.Join(lines, "\n") + "\n"
}
