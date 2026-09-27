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

// appendImportedTailscaleExitCandidates runs after user profile/overlay
// composition and before any OpenSurge-owned groups are generated. It adds a
// public Exit Node choice, not a Tailnet authorization rule. The original
// selector fields, provider membership and Mihomo selection behavior stay intact.
func appendImportedTailscaleExitCandidates(imported *importedProfile, cfg config.Config) error {
	if !cfg.Tailscale.Enabled || cfg.Tailscale.ExitNode == "" || imported.sections["proxy-groups"] == nil {
		return nil
	}
	// A select and an automatic group can share an anchored proxies list (or a
	// merged group template). Expand aliases into independent runtime nodes before
	// editing so the new choice cannot leak into the automatic group. Source YAML
	// is never rewritten, and retained fields/comments are preserved.
	remainingNodes := 100000
	for name, section := range imported.sections {
		cloned, err := cloneTailscaleCandidateNode(section, map[*yaml.Node]bool{}, &remainingNodes, false)
		if err != nil {
			return fmt.Errorf("prepare Tailscale Exit Node candidates in %s: %w", name, err)
		}
		imported.sections[name] = cloned
	}
	for _, group := range imported.sections["proxy-groups"].Content {
		var fields struct {
			Name    string   `yaml:"name"`
			Type    string   `yaml:"type"`
			Proxies []string `yaml:"proxies"`
		}
		if err := group.Decode(&fields); err != nil {
			return fmt.Errorf("read proxy group for Tailscale Exit Node candidates: %w", err)
		}
		if fields.Type != "select" || IsLocalRoutingGroup(fields.Name) || strings.HasPrefix(fields.Name, "device/") || fields.Name == config.TailscaleExitGroupName {
			continue
		}
		alreadyPresent := false
		for _, candidate := range fields.Proxies {
			if candidate == config.TailscaleExitGroupName {
				alreadyPresent = true
				break
			}
		}
		if alreadyPresent {
			continue
		}
		candidates := make([]*yaml.Node, 0, len(fields.Proxies)+1)
		for _, candidate := range fields.Proxies {
			candidates = append(candidates, quotedStringNode(candidate))
		}
		candidates = append(candidates, quotedStringNode(config.TailscaleExitGroupName))
		if index := mappingValueIndex(group, "proxies"); index >= 0 {
			group.Content[index] = sequenceNode(candidates...)
		} else {
			// An explicit field overrides a proxies list inherited through <<.
			group.Content = append(group.Content, stringNode("proxies"), sequenceNode(candidates...))
		}
	}
	return nil
}

func cloneTailscaleCandidateNode(node *yaml.Node, visiting map[*yaml.Node]bool, remainingNodes *int, fromAlias bool) (*yaml.Node, error) {
	if node == nil {
		return nil, nil
	}
	if visiting[node] {
		return nil, fmt.Errorf("recursive YAML alias is not supported")
	}
	if fromAlias {
		*remainingNodes -= 1
		if *remainingNodes < 0 {
			return nil, fmt.Errorf("expanded YAML exceeds the Tailscale candidate node limit")
		}
	}
	visiting[node] = true
	defer delete(visiting, node)
	if node.Kind == yaml.AliasNode {
		return cloneTailscaleCandidateNode(node.Alias, visiting, remainingNodes, true)
	}
	cloned := *node
	cloned.Anchor = ""
	cloned.Alias = nil
	cloned.Content = make([]*yaml.Node, 0, len(node.Content))
	for _, child := range node.Content {
		value, err := cloneTailscaleCandidateNode(child, visiting, remainingNodes, fromAlias)
		if err != nil {
			return nil, err
		}
		cloned.Content = append(cloned.Content, value)
	}
	return &cloned, nil
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
