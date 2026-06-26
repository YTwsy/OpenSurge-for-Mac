package config

import (
	"fmt"
	"net"
	"strings"
)

func Validate(cfg Config) error {
	if strings.TrimSpace(cfg.Gateway.Interface) == "" {
		return fmt.Errorf("gateway.interface is required")
	}
	if strings.TrimSpace(cfg.Gateway.UpstreamInterface) == "" {
		return fmt.Errorf("gateway.upstream_interface is required")
	}
	if net.ParseIP(cfg.Gateway.LANIP).To4() == nil {
		return fmt.Errorf("gateway.lan_ip must be a valid IPv4 address")
	}
	if cfg.DHCP.Enabled {
		if strings.TrimSpace(cfg.DHCP.Binary) == "" {
			return fmt.Errorf("dhcp.binary is required")
		}
		if net.ParseIP(cfg.DHCP.RangeStart).To4() == nil {
			return fmt.Errorf("dhcp.range_start must be a valid IPv4 address")
		}
		if net.ParseIP(cfg.DHCP.RangeEnd).To4() == nil {
			return fmt.Errorf("dhcp.range_end must be a valid IPv4 address")
		}
		if strings.TrimSpace(cfg.DHCP.LeaseTime) == "" {
			return fmt.Errorf("dhcp.lease_time is required")
		}
	}
	if net.ParseIP(cfg.DNS.Listen).To4() == nil {
		return fmt.Errorf("dns.listen must be a valid IPv4 address")
	}
	if !validPort(cfg.DNS.Port) {
		return fmt.Errorf("dns.port must be between 1 and 65535")
	}
	if strings.TrimSpace(cfg.Mihomo.Binary) == "" {
		return fmt.Errorf("mihomo.binary is required")
	}
	if strings.TrimSpace(cfg.Mihomo.Config) == "" {
		return fmt.Errorf("mihomo.config is required")
	}
	if !validPort(cfg.Mihomo.MixedPort) {
		return fmt.Errorf("mihomo.mixed_port must be between 1 and 65535")
	}
	if !validOptionalPort(cfg.Mihomo.RedirPort) {
		return fmt.Errorf("mihomo.redir_port must be between 0 and 65535")
	}
	if strings.TrimSpace(cfg.Mihomo.APIAddr) == "" {
		return fmt.Errorf("mihomo.api_addr is required")
	}
	if strings.TrimSpace(cfg.PF.AnchorName) == "" {
		return fmt.Errorf("pf.anchor_name is required")
	}
	if !validOptionalPort(cfg.PF.RedirectTCPTo) {
		return fmt.Errorf("pf.redirect_tcp_to must be between 0 and 65535")
	}
	if strings.TrimSpace(cfg.Runtime.Dir) == "" {
		return fmt.Errorf("runtime.dir is required")
	}
	return nil
}

func validPort(port int) bool {
	return port > 0 && port <= 65535
}

func validOptionalPort(port int) bool {
	return port >= 0 && port <= 65535
}
