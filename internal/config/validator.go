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
	if cfg.Mihomo.RedirPort != 0 {
		return fmt.Errorf("mihomo.redir_port is not supported on macOS; use transparent.mode: \"tun\"")
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
	if cfg.PF.RedirectTCPTo != 0 {
		return fmt.Errorf("pf.redirect_tcp_to is not supported on macOS; use transparent.mode: \"tun\"")
	}
	if err := validateTransparent(cfg.Transparent); err != nil {
		return err
	}
	if err := validateUpstreamProxy(cfg.UpstreamProxy); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Runtime.Dir) == "" {
		return fmt.Errorf("runtime.dir is required")
	}
	return nil
}

func validateTransparent(cfg TransparentConfig) error {
	switch cfg.Mode {
	case TransparentModeOff:
		return nil
	case TransparentModeTUN:
		if !strings.HasPrefix(cfg.TUNDevice, "utun") {
			return fmt.Errorf("transparent.tun_device must start with utun on macOS")
		}
		switch cfg.TUNStack {
		case "system", "gvisor", "mixed":
			return nil
		default:
			return fmt.Errorf("transparent.tun_stack must be system, gvisor, or mixed")
		}
	default:
		return fmt.Errorf("transparent.mode must be off or tun")
	}
}

func validateUpstreamProxy(cfg UpstreamProxyConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("upstream_proxy.name is required when upstream_proxy.enabled is true")
	}
	if strings.TrimSpace(cfg.Name) == "open-surge-egress" {
		return fmt.Errorf("upstream_proxy.name must differ from reserved proxy group open-surge-egress")
	}
	if !validRuleToken(cfg.Name) {
		return fmt.Errorf("upstream_proxy.name must not contain whitespace, commas, or control characters")
	}
	switch cfg.Type {
	case "http", "socks5":
	default:
		return fmt.Errorf("upstream_proxy.type must be http or socks5")
	}
	if strings.TrimSpace(cfg.Server) == "" {
		return fmt.Errorf("upstream_proxy.server is required when upstream_proxy.enabled is true")
	}
	if hasControlChar(cfg.Server) {
		return fmt.Errorf("upstream_proxy.server must not contain control characters")
	}
	if !validPort(cfg.Port) {
		return fmt.Errorf("upstream_proxy.port must be between 1 and 65535")
	}
	if !validDomainRule(cfg.MatchDomain) {
		return fmt.Errorf("upstream_proxy.match_domain must be a domain without scheme, path, whitespace, or comma")
	}
	if hasControlChar(cfg.Username) || hasControlChar(cfg.Password) {
		return fmt.Errorf("upstream_proxy credentials must not contain control characters")
	}
	return nil
}

func validRuleToken(value string) bool {
	if strings.TrimSpace(value) != value || value == "" {
		return false
	}
	if strings.ContainsAny(value, ",\t\n\r ") {
		return false
	}
	return !hasControlChar(value)
}

func validDomainRule(value string) bool {
	if !validRuleToken(value) {
		return false
	}
	return !strings.ContainsAny(value, "/:")
}

func hasControlChar(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func validPort(port int) bool {
	return port > 0 && port <= 65535
}

func validOptionalPort(port int) bool {
	return port >= 0 && port <= 65535
}
