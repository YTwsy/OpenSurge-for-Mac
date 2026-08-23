package config

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"

	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/lan"
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
	if cfg.Gateway.LANPrefixLen != 0 && !lan.ValidPrefixLen(cfg.Gateway.LANPrefixLen) {
		return fmt.Errorf("gateway.lan_prefix_len must be between %d and %d", lan.MinPrefixLen, lan.MaxPrefixLen)
	}
	scope, err := cfg.LANScope()
	if err != nil {
		return err
	}
	if !scope.UsableHost(scope.Gateway) {
		return fmt.Errorf("gateway.lan_ip must not be the %s network or broadcast address", scope)
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
		if err := validateDHCPRangeInLAN(cfg, scope); err != nil {
			return err
		}
	}
	if err := validateOptionalBypassAddresses(cfg.DHCP); err != nil {
		return err
	}
	if checkDevicePolicy {
		if err := validateDevicePolicy(cfg, scope); err != nil {
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
	if err := validateTailscale(cfg, scope, checkDevicePolicy); err != nil {
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
	if cfg.Transparent.IPv6Requested() && cfg.Gateway.SameLAN() && !cfg.Transparent.IPv6SharedL2Ready {
		if cfg.Gateway.Mode == GatewayModeSameLAN {
			return fmt.Errorf("transparent.tun_ipv6 in gateway.mode same_lan requires transparent.ipv6_shared_l2_ready: true after selected clients use the OpenSurge ULA, link-local default gateway, and DNS without a competing IPv6 default route")
		}
		return fmt.Errorf("transparent.tun_ipv6 in gateway.mode same_wifi_dhcp requires transparent.ipv6_shared_l2_ready: true after competing IPv6 RA/default routes have been removed from the shared LAN")
	}
	if cfg.LocalSystemProxy.Enabled && !cfg.Transparent.TUNEnabled() {
		return fmt.Errorf("local_system_proxy.enabled requires transparent.mode: \"tun\"")
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

func validateDevicePolicy(cfg Config, scope lan.Scope) error {
	if strings.TrimSpace(cfg.DevicePolicy.File) == "" {
		if len(cfg.DevicePolicy.ProtectedIPv4) > 0 {
			return fmt.Errorf("device_policy.protected_ipv4 requires device_policy.file")
		}
		return nil
	}
	bundle := cfg.DevicePolicy.Bundle
	if bundle == nil {
		var err error
		bundle, err = loadDevicePolicyBundle(cfg.DevicePolicy.File, scope, cfg.Gateway.Mode == GatewayModeSameLAN)
		if err != nil {
			return fmt.Errorf("device_policy.file: %w", err)
		}
	}
	activePolicy := device.ActivePolicySetForLAN(bundle.Policy, scope)
	if err := validateRouterBypass(cfg, scope, activePolicy); err != nil {
		return fmt.Errorf("device_policy.file: %w", err)
	}
	protected := append([]string(nil), cfg.DevicePolicy.ProtectedIPv4...)
	if device.UsesUpstreamRouter(activePolicy) {
		protected = append(protected, cfg.DHCP.BypassGateway)
	}
	if err := device.ValidatePolicySetForLAN(bundle.Policy, scope, protected, cfg.Gateway.Mode == GatewayModeSameLAN); err != nil {
		return fmt.Errorf("device_policy.file: %w", err)
	}
	return nil
}

// ValidateDevicePolicyCandidate applies the complete topology and DHCP
// contract to a candidate policy before the root helper writes it.
func ValidateDevicePolicyCandidate(cfg Config, policy device.PolicySet) error {
	scope, err := cfg.LANScope()
	if err != nil {
		return err
	}
	bundle, err := device.CompilePolicyBundleForLAN(policy, scope, cfg.Gateway.Mode == GatewayModeSameLAN)
	if err != nil {
		return err
	}
	candidate := cfg
	if strings.TrimSpace(candidate.DevicePolicy.File) == "" {
		candidate.DevicePolicy.File = "<candidate>"
	}
	candidate.DevicePolicy.Bundle = &bundle
	return validate(candidate, true)
}

func validateOptionalBypassAddresses(cfg DHCPConfig) error {
	if value := strings.TrimSpace(cfg.BypassGateway); value != "" && net.ParseIP(value).To4() == nil {
		return fmt.Errorf("dhcp.bypass_gateway must be a valid IPv4 address")
	}
	for _, value := range cfg.BypassDNS {
		if net.ParseIP(strings.TrimSpace(value)).To4() == nil {
			return fmt.Errorf("dhcp.bypass_dns entry %q must be a valid IPv4 address", value)
		}
	}
	return nil
}

func validateRouterBypass(cfg Config, scope lan.Scope, policy device.PolicySet) error {
	if !device.UsesUpstreamRouter(policy) {
		return nil
	}
	if cfg.Gateway.Mode != GatewayModeSameWiFiDHCP {
		return fmt.Errorf("gateway_target %q is only available in gateway.mode same_wifi_dhcp", device.GatewayTargetUpstreamRouter)
	}
	gateway := net.ParseIP(strings.TrimSpace(cfg.DHCP.BypassGateway)).To4()
	if gateway == nil {
		return fmt.Errorf("gateway_target %q requires dhcp.bypass_gateway", device.GatewayTargetUpstreamRouter)
	}
	if len(cfg.DHCP.BypassDNS) == 0 {
		return fmt.Errorf("gateway_target %q requires at least one dhcp.bypass_dns address", device.GatewayTargetUpstreamRouter)
	}
	// Bypassed clients keep their OpenSurge lease, so the router they are sent
	// to has to be reachable on-link from the same LAN.
	if !scope.Contains(gateway) {
		return fmt.Errorf("dhcp.bypass_gateway %s must remain in gateway LAN %s", gateway, scope)
	}
	if !scope.UsableHost(gateway) || gateway.Equal(scope.Gateway) {
		return fmt.Errorf("dhcp.bypass_gateway must be a usable host address different from gateway.lan_ip")
	}
	start := net.ParseIP(cfg.DHCP.RangeStart).To4()
	end := net.ParseIP(cfg.DHCP.RangeEnd).To4()
	if start != nil && end != nil && bytesCompareIPv4(gateway, start) >= 0 && bytesCompareIPv4(gateway, end) <= 0 {
		return fmt.Errorf("dhcp.bypass_gateway must not be inside the DHCP range")
	}
	return nil
}

func bytesCompareIPv4(left, right net.IP) int {
	for index := 0; index < net.IPv4len; index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}

// PrepareDevicePolicy loads and compiles the policy exactly once for a config
// instance. Gateway startup, DHCP rendering and mihomo rendering all consume
// the resulting immutable bundle.
func PrepareDevicePolicy(cfg *Config) error {
	if strings.TrimSpace(cfg.DevicePolicy.File) == "" {
		return nil
	}
	scope, err := cfg.LANScope()
	if err != nil {
		return err
	}
	ipOnlyDevicesActive := cfg.Gateway.Mode == GatewayModeSameLAN
	if cfg.DevicePolicy.Bundle != nil && cfg.DevicePolicy.Bundle.IPOnlyDevicesActive == ipOnlyDevicesActive && cfg.DevicePolicy.Bundle.ActiveLAN == scope.String() {
		return nil
	}
	if cfg.DevicePolicy.Bundle != nil {
		bundle, err := device.CompilePolicyBundleForLAN(cfg.DevicePolicy.Bundle.Policy, scope, ipOnlyDevicesActive)
		if err != nil {
			return fmt.Errorf("device_policy.file: %w", err)
		}
		cfg.DevicePolicy.Bundle = &bundle
		return nil
	}
	bundle, err := loadDevicePolicyBundle(cfg.DevicePolicy.File, scope, ipOnlyDevicesActive)
	if err != nil {
		return fmt.Errorf("device_policy.file: %w", err)
	}
	cfg.DevicePolicy.Bundle = bundle
	return nil
}

func loadDevicePolicyBundle(path string, scope lan.Scope, ipOnlyDevicesActive bool) (*device.PolicyBundle, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("must not be a directory")
	}
	bundle, err := device.LoadPolicyBundleForLAN(path, scope, ipOnlyDevicesActive)
	if err != nil {
		return nil, err
	}
	return &bundle, nil
}

// validateDHCPRangeInLAN keeps the pool inside the LAN dnsmasq actually serves.
// dnsmasq refuses a range that is not on one of its interface subnets, so this
// applies to every DHCP-owning topology rather than only same_wifi_dhcp.
func validateDHCPRangeInLAN(cfg Config, scope lan.Scope) error {
	start := net.ParseIP(cfg.DHCP.RangeStart).To4()
	end := net.ParseIP(cfg.DHCP.RangeEnd).To4()
	if start == nil || end == nil {
		return fmt.Errorf("dhcp.enabled requires IPv4 dhcp.range_start and dhcp.range_end")
	}
	if !scope.Contains(start) || !scope.Contains(end) {
		return fmt.Errorf("dhcp.enabled requires the DHCP range to remain in gateway LAN %s", scope)
	}
	if bytesCompareIPv4(start, end) > 0 {
		return fmt.Errorf("dhcp.range_start must not be after dhcp.range_end")
	}
	if !scope.UsableHost(start) || !scope.UsableHost(end) {
		return fmt.Errorf("the DHCP range must not include the %s network or broadcast address", scope)
	}
	if bytesCompareIPv4(scope.Gateway, start) >= 0 && bytesCompareIPv4(scope.Gateway, end) <= 0 {
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
	switch cfg.TUNIPv6 {
	case TUNIPv6Off, TUNIPv6Auto, TUNIPv6Always:
	default:
		return fmt.Errorf("transparent.tun_ipv6 must be off, auto, or always")
	}
	if cfg.TUNIPv6 != TUNIPv6Off {
		if cfg.Mode != TransparentModeTUN {
			return fmt.Errorf("transparent.tun_ipv6 %s requires transparent.mode: \"tun\"", cfg.TUNIPv6)
		}
		if strings.TrimSpace(cfg.IPv6PacketBrokerBinary) == "" {
			return fmt.Errorf("transparent.ipv6_packet_broker_binary is required when downstream IPv6 takeover is enabled")
		}
		if cfg.IPv6PacketMTU < 1280 || cfg.IPv6PacketMTU > 9000 {
			return fmt.Errorf("transparent.ipv6_packet_mtu must be between 1280 and 9000")
		}
	}
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
	if strings.TrimSpace(cfg.Name) == "open-surge-egress" || strings.HasPrefix(strings.TrimSpace(cfg.Name), "open-surge/mac-") {
		return fmt.Errorf("upstream_proxy.name must differ from reserved OpenSurge proxy groups")
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

func validateTailscale(cfg Config, scope lan.Scope, checkDevicePolicy bool) error {
	ts := cfg.Tailscale
	if !ts.Enabled {
		return nil
	}
	if strings.TrimSpace(ts.DisplayName) == "" || hasControlChar(ts.DisplayName) {
		return fmt.Errorf("tailscale.display_name must be a non-empty label without control characters")
	}
	if len([]rune(ts.DisplayName)) > 64 {
		return fmt.Errorf("tailscale.display_name must not exceed 64 characters")
	}
	if !validDNSLabel(ts.Hostname) {
		return fmt.Errorf("tailscale.hostname must be a DNS label using letters, numbers, and hyphens")
	}
	if err := validateTailscaleControlURL(ts.ControlURL); err != nil {
		return err
	}
	if strings.TrimSpace(ts.AuthKeyFile) == "" {
		return fmt.Errorf("tailscale.auth_key_file is required")
	}
	if strings.TrimSpace(ts.StateDir) == "" {
		return fmt.Errorf("tailscale.state_dir is required")
	}
	if ts.ExitNodeAllowLANAccess && strings.TrimSpace(ts.ExitNode) == "" {
		return fmt.Errorf("tailscale.exit_node_allow_lan_access requires tailscale.exit_node")
	}
	if ts.ExitNode != "" && !validRuleToken(ts.ExitNode) {
		return fmt.Errorf("tailscale.exit_node must not contain whitespace, commas, or control characters")
	}
	if ts.AllowAllDevices && len(ts.AllowedDevices) > 0 {
		return fmt.Errorf("tailscale.allowed_devices must be empty when tailscale.allow_all_devices is true")
	}
	for _, suffix := range ts.MagicDNSSuffixes {
		if !validDomainRule(suffix) || strings.HasPrefix(suffix, ".") || strings.Contains(suffix, "*") {
			return fmt.Errorf("tailscale.magic_dns_suffixes entry %q must be a domain suffix without wildcard", suffix)
		}
	}
	if err := validateTailscaleCIDRs("peer_cidrs", ts.PeerCIDRs, false, scope); err != nil {
		return err
	}
	if err := validateTailscaleCIDRs("subnet_routes", ts.SubnetRoutes, true, scope); err != nil {
		return err
	}
	if len(ts.SubnetRoutes) > 0 && !ts.AcceptRoutes {
		return fmt.Errorf("tailscale.subnet_routes requires tailscale.accept_routes: true")
	}
	if !checkDevicePolicy {
		return nil
	}
	if (ts.AllowAllDevices || len(ts.AllowedDevices) > 0) && cfg.DevicePolicy.Bundle == nil {
		return fmt.Errorf("tailscale device access requires device_policy.file")
	}
	if cfg.DevicePolicy.Bundle != nil {
		available := make(map[string]bool, len(cfg.DevicePolicy.Bundle.Compiled.Devices))
		for _, managed := range cfg.DevicePolicy.Bundle.Compiled.Devices {
			available[managed.ID] = true
		}
		for _, id := range ts.AllowedDevices {
			if !validRuleToken(id) {
				return fmt.Errorf("tailscale.allowed_devices entry %q is not a valid device ID", id)
			}
			if !available[id] {
				return fmt.Errorf("tailscale.allowed_devices references unknown or inactive device %q", id)
			}
		}
	}
	return nil
}

func validateTailscaleControlURL(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("tailscale.control_url must be an HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("tailscale.control_url must not contain credentials, query, or fragment")
	}
	return nil
}

func validateTailscaleCIDRs(field string, values []string, privateOnly bool, scope lan.Scope) error {
	seen := map[netip.Prefix]bool{}
	lanPrefix, err := netip.ParsePrefix(scope.String())
	if err != nil {
		return err
	}
	for _, value := range values {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil || prefix != prefix.Masked() || prefix.String() != strings.TrimSpace(value) {
			return fmt.Errorf("tailscale.%s entry %q must be a canonical IP CIDR", field, value)
		}
		if !prefix.Addr().IsGlobalUnicast() {
			return fmt.Errorf("tailscale.%s entry %q must be a unicast network", field, value)
		}
		if field == "peer_cidrs" && tailscaleAddressSpaceNeedsExactHost(prefix) {
			return fmt.Errorf("tailscale.peer_cidrs entry %q must identify one exact Tailscale peer (/32 for IPv4 or /128 for IPv6); broad Tailnet address-space capture is not allowed", value)
		}
		if privateOnly && !prefix.Addr().IsPrivate() {
			return fmt.Errorf("tailscale.%s entry %q must be a private IPv4/ULA subnet", field, value)
		}
		if prefix.Addr().Is4() && prefix.Overlaps(lanPrefix) {
			return fmt.Errorf("tailscale.%s entry %q overlaps the OpenSurge LAN %s", field, value, scope)
		}
		if seen[prefix] {
			return fmt.Errorf("tailscale.%s contains duplicate CIDR %q", field, value)
		}
		seen[prefix] = true
	}
	return nil
}

func tailscaleAddressSpaceNeedsExactHost(prefix netip.Prefix) bool {
	addressSpace := netip.MustParsePrefix("100.64.0.0/10")
	wantBits := 32
	if prefix.Addr().Is6() {
		addressSpace = netip.MustParsePrefix("fd7a:115c:a1e0::/48")
		wantBits = 128
	}
	return prefix.Overlaps(addressSpace) && prefix.Bits() != wantBits
}

func validDNSLabel(value string) bool {
	if value == "" || len(value) > 63 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
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
