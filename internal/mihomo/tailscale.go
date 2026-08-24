package mihomo

import (
	"fmt"
	"net/netip"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
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
	if len(sources) == 0 {
		return nil
	}
	rules := make([]string, 0, len(sources)*(len(cfg.Tailscale.MagicDNSSuffixes)+len(cfg.Tailscale.PeerCIDRs)+len(cfg.Tailscale.SubnetRoutes)))
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

func renderTailscaleRouteAddresses(cfg config.Config) string {
	if !cfg.Tailscale.Enabled || !cfg.Transparent.TUNEnabled() {
		return ""
	}
	values := append(append([]string(nil), cfg.Tailscale.PeerCIDRs...), cfg.Tailscale.SubnetRoutes...)
	if len(values) == 0 {
		return ""
	}
	var out strings.Builder
	for _, value := range values {
		out.WriteString("    - " + value + "\n")
	}
	return strings.TrimRight(out.String(), "\n")
}
