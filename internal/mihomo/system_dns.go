package mihomo

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
)

func coordinateDNSResolvers(cfg config.Config, fields string) (string, error) {
	if !cfg.ManageSystemDNS() {
		return fields, nil
	}
	root, err := decodeSingleYAMLMapping([]byte(fields))
	if err != nil {
		return "", err
	}
	// After macOS DNS points into TUN, a system resolver in the engine would
	// point back to the same engine. Reject that cycle before touching the host.
	var check func(*yaml.Node) error
	seen := map[*yaml.Node]bool{}
	check = func(node *yaml.Node) error {
		node = resolveAlias(node)
		if node == nil || seen[node] {
			return nil
		}
		seen[node] = true
		value := strings.ToLower(strings.TrimSpace(node.Value))
		if node.Kind == yaml.ScalarNode && (value == "system" || strings.HasPrefix(value, "system://") || strings.HasPrefix(value, "system#") || value == "dhcp://system") {
			return fmt.Errorf("local_system_dns cannot use a mihomo system resolver; configure explicit DNS upstreams in the profile or disable local_system_dns.enabled")
		}
		for index, child := range node.Content {
			if node.Kind == yaml.MappingNode && index%2 == 0 {
				continue
			}
			if err := check(child); err != nil {
				return err
			}
		}
		return nil
	}
	for _, field := range []string{"nameserver", "default-nameserver", "fallback", "proxy-server-nameserver", "direct-nameserver", "nameserver-policy", "proxy-server-nameserver-policy"} {
		if index := mappingValueIndex(root, field); index >= 0 {
			if err := check(root.Content[index]); err != nil {
				return "", err
			}
		}
	}
	var resolvers []string
	for _, server := range cfg.LocalSystemDNS.Resolvers {
		addr, err := netip.ParseAddr(server)
		if err != nil {
			return "", fmt.Errorf("invalid original DNS server %q", server)
		}
		// Never feed our own DNS/TUN endpoint back into private resolution.
		if addr.IsLoopback() || server == config.LocalSystemDNSServer || server == cfg.DNS.Listen || netip.MustParsePrefix("198.18.0.0/16").Contains(addr) || netip.MustParsePrefix(config.MihomoFakeIPv6Range).Contains(addr) {
			continue
		}
		if addr.Is6() {
			server = "udp://[" + strings.ReplaceAll(server, "%", "%25") + "]:53"
		}
		resolvers = append(resolvers, server)
	}
	if len(resolvers) == 0 {
		return fields, nil
	}
	domains := []string{"lan", "home.arpa", "10.in-addr.arpa", "168.192.in-addr.arpa", "d.f.ip6.arpa", "c.f.ip6.arpa"}
	for n := 16; n <= 31; n++ {
		domains = append(domains, fmt.Sprintf("%d.172.in-addr.arpa", n))
	}
	for _, domain := range append(slices.Clone(cfg.LocalSystemDNS.Domains), cfg.DHCP.Domain) {
		domain = strings.Trim(strings.ToLower(domain), ". ")
		if domain == "" || domain == "local" || slices.Contains(domains, domain) {
			continue
		}
		if strings.ContainsAny(domain, "/,:* \n\r\t") {
			return "", fmt.Errorf("invalid private DNS domain %q", domain)
		}
		domains = append(domains, domain)
	}
	policy := dnsMapping(root, "nameserver-policy")
	if policy.Kind != yaml.MappingNode {
		return "", fmt.Errorf("nameserver-policy must be a mapping")
	}
	filter := dnsSequence(root, "fake-ip-filter")
	if filter.Kind != yaml.SequenceNode {
		return "", fmt.Errorf("fake-ip-filter must be a sequence")
	}
	mode := "blacklist"
	if index := mappingValueIndex(root, "fake-ip-filter-mode"); index >= 0 {
		mode = root.Content[index].Value
	}
	// Whitelist semantics stay intact; the explicit private resolver policy
	// still resolves those names through the captured LAN DNS if they are fake.
	var privateFilters []*yaml.Node
	for _, domain := range domains {
		key := "+." + domain
		if mappingValueIndex(policy, key) < 0 {
			servers := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			for _, value := range resolvers {
				servers.Content = append(servers.Content, dnsString(value))
			}
			policy.Content = append(policy.Content, dnsString(key), servers)
		}
		switch mode {
		case "", "blacklist":
			privateFilters = append(privateFilters, dnsString(key))
		case "rule":
			privateFilters = append(privateFilters, dnsString("DOMAIN-SUFFIX,"+domain+",real-ip"))
		}
	}
	filter.Content = append(privateFilters, filter.Content...)
	return encodeYAMLNode(root)
}

func dnsString(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}
func dnsMapping(root *yaml.Node, key string) *yaml.Node {
	return dnsField(root, key, yaml.MappingNode, "!!map")
}
func dnsSequence(root *yaml.Node, key string) *yaml.Node {
	return dnsField(root, key, yaml.SequenceNode, "!!seq")
}
func dnsField(root *yaml.Node, key string, kind yaml.Kind, tag string) *yaml.Node {
	if index := mappingValueIndex(root, key); index >= 0 {
		return root.Content[index]
	}
	value := &yaml.Node{Kind: kind, Tag: tag}
	root.Content = append(root.Content, dnsString(key), value)
	return value
}
