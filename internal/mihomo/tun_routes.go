package mihomo

import (
	"net/netip"
	"strings"

	"open-mihomo-gateway/internal/config"
)

func renderTUNRouteAddresses(cfg config.Config, lanPrefix string) string {
	if !cfg.Transparent.TUNEnabled() {
		return ""
	}
	var values []string
	if cfg.Tailscale.Enabled {
		values = append(append([]string(nil), cfg.Tailscale.PeerCIDRs...), cfg.Tailscale.SubnetRoutes...)
	}
	// route-address replaces Mihomo's normal Darwin auto-route defaults. Build
	// the ordinary public ranges with the standard exclusions already removed,
	// then append exact Tailnet routes. Keeping the explicit /32 or /128 as a
	// separate route lets the OpenSurge TUN outrank the native Tailscale route;
	// leaving route-exclude-address enabled would merge the overlapping ranges
	// into an IP set and silently discard that more-specific route.
	routes := []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}
	for _, excluded := range []string{
		"0.0.0.0/8",
		lanPrefix,
		"10.0.0.0/8",
		"127.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"224.0.0.0/4",
		"255.255.255.255/32",
	} {
		routes = excludeRoutePrefix(routes, netip.MustParsePrefix(excluded))
	}
	// Host IPv6 is independent of downstream RA/BPF and of DNS AAAA answers.
	// Keep local/link-local/multicast and downstream ULA traffic off the system
	// TUN. The full fake pool needs a route even without native upstream IPv6;
	// a custom Tailnet IPv6 route must never replace ordinary host capture.
	routes = append(routes, netip.MustParsePrefix("2000::/3"), netip.MustParsePrefix(config.MihomoFakeIPv6Range))
	var out strings.Builder
	for _, route := range routes {
		out.WriteString("    - " + route.String() + "\n")
	}
	for _, value := range values {
		out.WriteString("    - " + value + "\n")
	}
	return strings.TrimRight(out.String(), "\n")
}

func excludeRoutePrefix(routes []netip.Prefix, excluded netip.Prefix) []netip.Prefix {
	excluded = excluded.Masked()
	result := make([]netip.Prefix, 0, len(routes))
	for _, route := range routes {
		route = route.Masked()
		if route.Addr().BitLen() != excluded.Addr().BitLen() {
			result = append(result, route)
			continue
		}
		if excluded.Bits() <= route.Bits() && excluded.Contains(route.Addr()) {
			continue
		}
		if !route.Contains(excluded.Addr()) {
			result = append(result, route)
			continue
		}
		result = append(result, subtractContainedPrefix(route, excluded)...)
	}
	return result
}

func subtractContainedPrefix(route, excluded netip.Prefix) []netip.Prefix {
	if excluded.Bits() <= route.Bits() {
		return nil
	}
	left, right := splitRoutePrefix(route)
	if left.Contains(excluded.Addr()) {
		return append(subtractContainedPrefix(left, excluded), right)
	}
	return append([]netip.Prefix{left}, subtractContainedPrefix(right, excluded)...)
}

func splitRoutePrefix(prefix netip.Prefix) (netip.Prefix, netip.Prefix) {
	nextBits := prefix.Bits() + 1
	left := netip.PrefixFrom(prefix.Masked().Addr(), nextBits)
	bit := prefix.Bits()
	if prefix.Addr().Is4() {
		value := prefix.Masked().Addr().As4()
		value[bit/8] |= byte(1 << (7 - bit%8))
		return left, netip.PrefixFrom(netip.AddrFrom4(value), nextBits)
	}
	value := prefix.Masked().Addr().As16()
	value[bit/8] |= byte(1 << (7 - bit%8))
	return left, netip.PrefixFrom(netip.AddrFrom16(value), nextBits)
}
