package mihomo

import (
	"fmt"
	"net/netip"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
)

func renderManagedTailscaleProxy(cfg config.Config) (string, error) {
	if !cfg.Tailscale.Enabled {
		return "", nil
	}
	authKey, err := tailscaleAuthKey(cfg.Tailscale)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	out.WriteString("  - name: " + yamlQuote(config.TailscaleProxyName) + "\n")
	out.WriteString("    type: tailscale\n")
	out.WriteString("    hostname: " + yamlQuote(cfg.Tailscale.Hostname) + "\n")
	if authKey != "" {
		out.WriteString("    auth-key: " + yamlQuote(authKey) + "\n")
	}
	out.WriteString("    control-url: " + yamlQuote(cfg.Tailscale.ControlURL) + "\n")
	out.WriteString("    state-dir: " + yamlQuote(cfg.Tailscale.StateDir) + "\n")
	out.WriteString("    ephemeral: false\n")
	out.WriteString("    udp: true\n")
	out.WriteString(fmt.Sprintf("    accept-routes: %t\n", cfg.Tailscale.AcceptRoutes))
	if cfg.Tailscale.ExitNode != "" {
		out.WriteString("    exit-node: " + yamlQuote(cfg.Tailscale.ExitNode) + "\n")
		out.WriteString(fmt.Sprintf("    exit-node-allow-lan-access: %t\n", cfg.Tailscale.ExitNodeAllowLANAccess))
	}
	return out.String(), nil
}

func appendImportedTailscaleProxy(imported *importedProfile, cfg config.Config) error {
	if !cfg.Tailscale.Enabled {
		return nil
	}
	authKey, err := tailscaleAuthKey(cfg.Tailscale)
	if err != nil {
		return err
	}
	body := mappingNode(
		stringNode("name"), quotedStringNode(config.TailscaleProxyName),
		stringNode("type"), stringNode("tailscale"),
		stringNode("hostname"), quotedStringNode(cfg.Tailscale.Hostname),
	)
	if authKey != "" {
		body.Content = append(body.Content, stringNode("auth-key"), quotedStringNode(authKey))
	}
	body.Content = append(body.Content,
		stringNode("control-url"), quotedStringNode(cfg.Tailscale.ControlURL),
		stringNode("state-dir"), quotedStringNode(cfg.Tailscale.StateDir),
		stringNode("ephemeral"), boolNode(false),
		stringNode("udp"), boolNode(true),
		stringNode("accept-routes"), boolNode(cfg.Tailscale.AcceptRoutes),
	)
	if cfg.Tailscale.ExitNode != "" {
		body.Content = append(body.Content,
			stringNode("exit-node"), quotedStringNode(cfg.Tailscale.ExitNode),
			stringNode("exit-node-allow-lan-access"), boolNode(cfg.Tailscale.ExitNodeAllowLANAccess),
		)
	}
	section := ensureImportedSection(imported, "proxies", yaml.SequenceNode, "!!seq")
	section.Style &^= yaml.FlowStyle
	section.Content = append(section.Content, body)
	return nil
}

func tailscaleExitSelectorGroups(cfg config.Config) []device.SelectorGroup {
	if !cfg.Tailscale.Enabled || cfg.Tailscale.ExitNode == "" {
		return nil
	}
	return []device.SelectorGroup{{
		Name:     config.TailscaleExitGroupName,
		Policies: []string{config.TailscaleProxyName},
	}}
}

func tailscaleAuthKey(cfg config.TailscaleConfig) (string, error) {
	data, err := os.ReadFile(cfg.AuthKeyFile)
	if err != nil {
		if os.IsNotExist(err) && tailscaleIdentityPresent(cfg.StateDir) {
			return "", nil
		}
		if os.IsNotExist(err) {
			return "", fmt.Errorf("tailscale auth key is required before the first connection")
		}
		return "", fmt.Errorf("read tailscale auth key: %w", err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" || strings.ContainsAny(key, " \t\r\n") {
		return "", fmt.Errorf("tailscale auth key file does not contain a valid key")
	}
	return key, nil
}

func tailscaleIdentityPresent(stateDir string) bool {
	entries, err := os.ReadDir(stateDir)
	return err == nil && len(entries) > 0
}

func renderTailscaleRules(cfg config.Config, policy policySections) []string {
	if !cfg.Tailscale.Enabled {
		return nil
	}
	sources := tailscaleRuleSources(cfg, policy)
	targetCount := len(cfg.Tailscale.MagicDNSSuffixes) + len(cfg.Tailscale.PeerCIDRs) + len(cfg.Tailscale.SubnetRoutes)
	rules := make([]string, 0, (len(sources)+1)*targetCount)
	for _, source := range sources {
		for _, suffix := range cfg.Tailscale.MagicDNSSuffixes {
			rules = append(rules, renderTailscaleAccessRule(source, "DOMAIN-SUFFIX,"+suffix))
		}
		for _, cidr := range append(append([]string(nil), cfg.Tailscale.PeerCIDRs...), cfg.Tailscale.SubnetRoutes...) {
			prefix, _ := netip.ParsePrefix(cidr)
			typeName := "IP-CIDR"
			if prefix.Addr().Is6() {
				typeName = "IP-CIDR6"
			}
			rules = append(rules, renderTailscaleAccessRule(source, typeName+","+cidr))
		}
	}
	// The host can also run the native Tailscale App, which installs system
	// routes for the same peers and subnets. Once all explicitly authorized
	// sources have had a chance to select the managed tsnet outbound, reject
	// every configured Tailnet target before ordinary CGNAT/RFC1918 DIRECT
	// protection rules. Otherwise an unauthorized downstream source could fall
	// through to DIRECT and be forwarded by the host's native Tailscale route.
	for _, suffix := range cfg.Tailscale.MagicDNSSuffixes {
		rules = append(rules, "DOMAIN-SUFFIX,"+suffix+",REJECT")
	}
	for _, cidr := range append(append([]string(nil), cfg.Tailscale.PeerCIDRs...), cfg.Tailscale.SubnetRoutes...) {
		prefix, _ := netip.ParsePrefix(cidr)
		typeName := "IP-CIDR"
		if prefix.Addr().Is6() {
			typeName = "IP-CIDR6"
		}
		rules = append(rules, typeName+","+cidr+",REJECT")
	}
	return rules
}

type tailscaleRuleSource []string

func renderTailscaleAccessRule(source tailscaleRuleSource, destination string) string {
	clauses := append(append(tailscaleRuleSource(nil), source...), destination)
	return "AND,((" + strings.Join(clauses, "),(") + "))," + config.TailscaleProxyName
}

func tailscaleRuleSources(cfg config.Config, policy policySections) []tailscaleRuleSource {
	sources := []tailscaleRuleSource{}
	if cfg.Tailscale.AllowMac {
		for _, inbound := range localRoutingInbounds(cfg) {
			sources = append(sources, tailscaleRuleSource(append([]string(nil), inbound.match...)))
		}
	}
	if policy.bundle == nil {
		return sources
	}
	selected := map[string]bool{}
	for _, id := range cfg.Tailscale.AllowedDevices {
		selected[id] = true
	}
	for _, managed := range policy.bundle.Compiled.Devices {
		if !cfg.Tailscale.AllowAllDevices && !selected[managed.ID] {
			continue
		}
		sources = append(sources, tailscaleRuleSource{"SRC-IP-CIDR," + managed.IPv4 + "/32"})
		if policy.ipv6 && managed.MAC != "" {
			sources = append(sources, tailscaleRuleSource{"IN-USER," + DeviceInboundUser(managed.ID)})
		}
	}
	return sources
}

func renderTailscaleRouteAddresses(cfg config.Config, lanPrefix string) string {
	if !cfg.Tailscale.Enabled || !cfg.Transparent.TUNEnabled() {
		return ""
	}
	values := append(append([]string(nil), cfg.Tailscale.PeerCIDRs...), cfg.Tailscale.SubnetRoutes...)
	if len(values) == 0 {
		return ""
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
	if cfg.Transparent.TUNIPv6 != config.TUNIPv6Off {
		ipv6Routes := excludeRoutePrefix(
			[]netip.Prefix{netip.MustParsePrefix("::/0")},
			netip.MustParsePrefix("::/8"),
		)
		routes = append(routes, ipv6Routes...)
	}
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
