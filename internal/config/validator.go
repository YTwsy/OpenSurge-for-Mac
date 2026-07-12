package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"open-mihomo-gateway/internal/device"
)

func Validate(cfg Config) error {
	return validate(cfg, true)
}

// ValidateRuntime omits the mutable desired device-policy document. It is for
// operations that consume the persisted applied snapshot rather than deploying
// a new policy.
func ValidateRuntime(cfg Config) error {
	return validate(cfg, false)
}

func validate(cfg Config, checkDevicePolicy bool) error {
	switch cfg.Gateway.Mode {
	case GatewayModeIsolatedLAN, GatewayModeSameLAN, GatewayModeSameWiFiDHCP:
	default:
		return fmt.Errorf("gateway.mode must be isolated_lan, same_lan, or same_wifi_dhcp")
	}
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
	if checkDevicePolicy {
		if err := validateDevicePolicy(cfg); err != nil {
			return err
		}
	}
	if net.ParseIP(cfg.DNS.Listen).To4() == nil {
		return fmt.Errorf("dns.listen must be a valid IPv4 address")
	}
	if !validPort(cfg.DNS.Port) {
		return fmt.Errorf("dns.port must be between 1 and 65535")
	}
	if err := validateDNSUpstream(cfg.DNS.Upstream); err != nil {
		return err
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
	if err := validateMihomoProfile(cfg); err != nil {
		return err
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
	if cfg.Gateway.SameLAN() {
		if cfg.Transparent.Mode != TransparentModeTUN {
			return fmt.Errorf("gateway.mode %s requires transparent.mode: \"tun\"", cfg.Gateway.Mode)
		}
	}
	switch cfg.Gateway.Mode {
	case GatewayModeSameLAN:
		if cfg.DHCP.Enabled {
			return fmt.Errorf("gateway.mode same_lan requires dhcp.enabled: false")
		}
	case GatewayModeSameWiFiDHCP:
		if !cfg.DHCP.Enabled {
			return fmt.Errorf("gateway.mode same_wifi_dhcp requires dhcp.enabled: true")
		}
		if err := validateDHCPRangeInLAN(cfg); err != nil {
			return err
		}
	}
	if err := validateUpstreamProxy(cfg.UpstreamProxy); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Runtime.Dir) == "" {
		return fmt.Errorf("runtime.dir is required")
	}
	return nil
}

func validateDNSUpstream(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		// Older installed configs used an empty value. dnsmasq rendering maps it
		// to MihomoDNSUpstream so upgrades do not silently keep system-resolver
		// behavior.
		return nil
	}
	host, portText, hasPort := strings.Cut(value, "#")
	if net.ParseIP(host).To4() == nil {
		return fmt.Errorf("dns.upstream must be an IPv4 address or IPv4#port")
	}
	if !hasPort {
		return nil
	}
	if strings.Contains(portText, "#") {
		return fmt.Errorf("dns.upstream must be an IPv4 address or IPv4#port")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || !validPort(port) {
		return fmt.Errorf("dns.upstream port must be between 1 and 65535")
	}
	return nil
}

func validateDevicePolicy(cfg Config) error {
	if strings.TrimSpace(cfg.DevicePolicy.File) == "" {
		if len(cfg.DevicePolicy.ProtectedIPv4) > 0 {
			return fmt.Errorf("device_policy.protected_ipv4 requires device_policy.file")
		}
		return nil
	}
	bundle := cfg.DevicePolicy.Bundle
	if bundle == nil {
		var err error
		bundle, err = loadDevicePolicyBundle(cfg.DevicePolicy.File)
		if err != nil {
			return fmt.Errorf("device_policy.file: %w", err)
		}
	}
	if err := device.ValidatePolicySetForLANWithProtected(bundle.Policy, cfg.Gateway.LANIP, cfg.DevicePolicy.ProtectedIPv4); err != nil {
		return fmt.Errorf("device_policy.file: %w", err)
	}
	return nil
}

// PrepareDevicePolicy loads and compiles the policy exactly once for a config
// instance. Gateway startup, DHCP rendering and mihomo rendering all consume
// the resulting immutable bundle.
func PrepareDevicePolicy(cfg *Config) error {
	if strings.TrimSpace(cfg.DevicePolicy.File) == "" || cfg.DevicePolicy.Bundle != nil {
		return nil
	}
	bundle, err := loadDevicePolicyBundle(cfg.DevicePolicy.File)
	if err != nil {
		return fmt.Errorf("device_policy.file: %w", err)
	}
	cfg.DevicePolicy.Bundle = bundle
	return nil
}

func loadDevicePolicyBundle(path string) (*device.PolicyBundle, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("must not be a directory")
	}
	bundle, err := device.LoadPolicyBundle(path)
	if err != nil {
		return nil, err
	}
	return &bundle, nil
}

func validateDHCPRangeInLAN(cfg Config) error {
	lanIP := cfg.LANIP().To4()
	start := net.ParseIP(cfg.DHCP.RangeStart).To4()
	end := net.ParseIP(cfg.DHCP.RangeEnd).To4()
	if lanIP == nil || start == nil || end == nil {
		return fmt.Errorf("same_wifi_dhcp requires IPv4 LAN and DHCP range addresses")
	}
	if lanIP[0] != start[0] || lanIP[1] != start[1] || lanIP[2] != start[2] ||
		lanIP[0] != end[0] || lanIP[1] != end[1] || lanIP[2] != end[2] {
		return fmt.Errorf("gateway.mode same_wifi_dhcp requires the DHCP range to remain in %d.%d.%d.0/24", lanIP[0], lanIP[1], lanIP[2])
	}
	if start[3] > end[3] {
		return fmt.Errorf("dhcp.range_start must not be after dhcp.range_end")
	}
	if start[3] == 0 || end[3] == 255 {
		return fmt.Errorf("same_wifi_dhcp DHCP range must not include the network or broadcast address")
	}
	if lanIP[3] >= start[3] && lanIP[3] <= end[3] {
		return fmt.Errorf("gateway.lan_ip must not be inside the DHCP range")
	}
	return nil
}

func validateMihomoProfile(cfg Config) error {
	switch cfg.Mihomo.ProfileMode {
	case MihomoProfileModeManaged:
		if strings.TrimSpace(cfg.Mihomo.Profile) != "" {
			return fmt.Errorf("mihomo.profile requires mihomo.profile_mode: \"imported\"")
		}
	case MihomoProfileModeImported:
		if strings.TrimSpace(cfg.Mihomo.Profile) == "" {
			return fmt.Errorf("mihomo.profile is required when mihomo.profile_mode is imported")
		}
		if cfg.UpstreamProxy.Enabled {
			return fmt.Errorf("upstream_proxy.enabled cannot be true when mihomo.profile_mode is imported")
		}
	default:
		return fmt.Errorf("mihomo.profile_mode must be managed or imported")
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
