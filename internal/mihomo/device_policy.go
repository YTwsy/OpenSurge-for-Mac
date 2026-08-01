package mihomo

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
)

type policySections struct {
	bundle    *device.PolicyBundle
	groups    []device.SelectorGroup
	providers []device.RuleProvider
	preRules  []string
	dedicated []string
	defaults  []string
}

func renderPolicySections(cfg config.Config, imported *importedProfile) (string, error) {
	sections, err := loadPolicySections(cfg.DevicePolicy.Bundle, cfg.DevicePolicy.File)
	if err != nil {
		return "", err
	}
	if cfg.Mihomo.ProfileMode == config.MihomoProfileModeImported {
		if imported == nil {
			return "", fmt.Errorf("imported mihomo profile was not loaded")
		}
		if err := validateImportedPolicySections(imported.inventory, sections); err != nil {
			return "", err
		}
		return composeImportedPolicySections(imported, sections)
	}
	if err := validateManagedPolicySections(cfg, sections); err != nil {
		return "", err
	}
	return composeManagedPolicySections(cfg, sections), nil
}

func loadPolicySections(bundle *device.PolicyBundle, path string) (policySections, error) {
	if bundle == nil && strings.TrimSpace(path) == "" {
		return policySections{}, nil
	}
	if bundle == nil {
		loaded, err := device.LoadPolicyBundle(path)
		if err != nil {
			return policySections{}, err
		}
		bundle = &loaded
	}
	return policySections{
		bundle:    bundle,
		groups:    bundle.Compiled.SelectorGroups,
		providers: bundle.Compiled.RuleProviders,
		preRules:  bundle.Compiled.OverrideRules,
		dedicated: bundle.Compiled.DedicatedRules,
		defaults:  bundle.Compiled.DefaultRules,
	}, nil
}

func composeManagedPolicySections(cfg config.Config, policy policySections) string {
	var out strings.Builder
	if cfg.UpstreamProxy.Enabled {
		out.WriteString("proxies:\n")
		out.WriteString("  - name: " + yamlQuote(cfg.UpstreamProxy.Name) + "\n")
		out.WriteString("    type: " + cfg.UpstreamProxy.Type + "\n")
		out.WriteString("    server: " + yamlQuote(cfg.UpstreamProxy.Server) + "\n")
		out.WriteString(fmt.Sprintf("    port: %d\n", cfg.UpstreamProxy.Port))
		if cfg.UpstreamProxy.Username != "" {
			out.WriteString("    username: " + yamlQuote(cfg.UpstreamProxy.Username) + "\n")
		}
		if cfg.UpstreamProxy.Password != "" {
			out.WriteString("    password: " + yamlQuote(cfg.UpstreamProxy.Password) + "\n")
		}
		out.WriteString("\n")
	} else {
		out.WriteString("proxies: []\n\n")
	}

	managedGroups := []device.SelectorGroup(nil)
	if cfg.UpstreamProxy.Enabled {
		managedGroups = append(managedGroups, device.SelectorGroup{Name: "open-surge-egress", Policies: []string{cfg.UpstreamProxy.Name}})
	}
	managedGroups = append(managedGroups, policy.groups...)
	if len(managedGroups) > 0 {
		out.WriteString(renderSelectorGroups(managedGroups))
		out.WriteString("\n")
	}
	if len(policy.providers) > 0 {
		out.WriteString(renderRuleProviders(policy.providers))
		out.WriteString("\n")
	}

	rules := orderedDevicePreRules(policy)
	if cfg.UpstreamProxy.Enabled {
		rules = append(rules, "DOMAIN,"+cfg.UpstreamProxy.MatchDomain+",open-surge-egress")
	}
	rules = append(rules, policy.defaults...)
	rules = append(rules, "MATCH,DIRECT")
	out.WriteString("rules:\n")
	writeRuleLines(&out, rules)
	return out.String()
}

func composeImportedPolicySections(imported *importedProfile, policy policySections) (string, error) {
	if len(policy.groups) > 0 {
		appendImportedSelectorGroups(imported, policy.groups)
	}
	if len(policy.providers) > 0 {
		appendImportedRuleProviders(imported, policy.providers)
	}
	if err := composeImportedRules(imported.sections["rules"], orderedDevicePreRules(policy), policy.defaults); err != nil {
		return "", err
	}
	return renderImportedProfileSections(imported)
}

func appendImportedSelectorGroups(imported *importedProfile, groups []device.SelectorGroup) {
	section := ensureImportedSection(imported, "proxy-groups", yaml.SequenceNode, "!!seq")
	section.Style &^= yaml.FlowStyle
	for _, group := range groups {
		policies := make([]*yaml.Node, 0, len(group.Policies))
		for _, policy := range group.Policies {
			policies = append(policies, quotedStringNode(policy))
		}
		section.Content = append(section.Content, mappingNode(
			stringNode("name"), stringNode(group.Name),
			stringNode("type"), stringNode("select"),
			stringNode("proxies"), sequenceNode(policies...),
		))
	}
}

func appendImportedRuleProviders(imported *importedProfile, providers []device.RuleProvider) {
	section := ensureImportedSection(imported, "rule-providers", yaml.MappingNode, "!!map")
	section.Style &^= yaml.FlowStyle
	for _, provider := range providers {
		body := mappingNode(
			stringNode("type"), stringNode(provider.Type),
			stringNode("behavior"), stringNode(provider.Behavior),
		)
		if provider.Type == "http" {
			body.Content = append(body.Content,
				stringNode("url"), quotedStringNode(provider.URL),
				stringNode("format"), stringNode(provider.Format),
			)
			if provider.Interval > 0 {
				body.Content = append(body.Content, stringNode("interval"), intNode(provider.Interval))
			}
		} else {
			payload := make([]*yaml.Node, 0, len(provider.Payload))
			for _, value := range provider.Payload {
				payload = append(payload, quotedStringNode(value))
			}
			body.Content = append(body.Content, stringNode("payload"), sequenceNode(payload...))
		}
		section.Content = append(section.Content, stringNode(provider.Name), body)
	}
}

func ensureImportedSection(imported *importedProfile, name string, kind yaml.Kind, tag string) *yaml.Node {
	if existing := imported.sections[name]; existing != nil {
		return existing
	}
	section := &yaml.Node{Kind: kind, Tag: tag}
	imported.sections[name] = section
	imported.sectionKeys[name] = stringNode(name)
	insertAt := len(imported.sectionOrder)
	for i, sectionName := range imported.sectionOrder {
		if sectionName == "rules" {
			insertAt = i
			break
		}
	}
	imported.sectionOrder = append(imported.sectionOrder, "")
	copy(imported.sectionOrder[insertAt+1:], imported.sectionOrder[insertAt:])
	imported.sectionOrder[insertAt] = name
	return section
}

// composeImportedRules inserts system and device override rules before global
// rules. Legacy device defaults remain immediately before a terminal MATCH.
func composeImportedRules(rules *yaml.Node, preRules, defaultRules []string) error {
	if err := validateImportedRules(rules); err != nil {
		return err
	}
	terminalIndex := -1
	if len(rules.Content) > 0 {
		if value, ok := scalarStringValue(rules.Content[len(rules.Content)-1]); ok && isTerminalMatchValue(value) {
			terminalIndex = len(rules.Content) - 1
		}
	}
	before := rules.Content
	var terminal []*yaml.Node
	if terminalIndex >= 0 {
		before = rules.Content[:terminalIndex]
		terminal = rules.Content[terminalIndex:]
	}
	content := make([]*yaml.Node, 0, len(preRules)+len(before)+len(defaultRules)+len(terminal))
	content = append(content, ruleNodes(preRules)...)
	content = append(content, before...)
	content = append(content, ruleNodes(defaultRules)...)
	content = append(content, terminal...)
	rules.Content = content
	if len(preRules) > 0 || len(defaultRules) > 0 {
		rules.Style &^= yaml.FlowStyle
	}
	return nil
}

func ruleNodes(rules []string) []*yaml.Node {
	nodes := make([]*yaml.Node, 0, len(rules))
	for _, rule := range rules {
		nodes = append(nodes, stringNode(rule))
	}
	return nodes
}

var dedicatedLocalCIDRs = []string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"224.0.0.0/4",
}

// Dedicated device egress is a public-Internet routing choice. Keep local,
// link-local, carrier-grade NAT, and multicast destinations direct before any
// device-owned override or catch-all selector so gateway and LAN access cannot
// be accidentally sent to a remote proxy.
func orderedDevicePreRules(policy policySections) []string {
	rules := []string{}
	if policy.bundle != nil {
		for _, managed := range policy.bundle.Compiled.Devices {
			if managed.EgressMode != device.EgressModeDedicated {
				continue
			}
			for _, cidr := range dedicatedLocalCIDRs {
				rules = append(rules, fmt.Sprintf("AND,((SRC-IP-CIDR,%s/32),(IP-CIDR,%s)),DIRECT", managed.IPv4, cidr))
			}
		}
	}
	rules = append(rules, policy.preRules...)
	rules = append(rules, policy.dedicated...)
	return rules
}

func validateImportedPolicySections(inventory importedProfileInventory, policy policySections) error {
	if policy.bundle == nil {
		return nil
	}
	for name := range inventory.targets {
		if strings.HasPrefix(name, "device/") {
			return fmt.Errorf("imported mihomo profile target %q occupies reserved device/ namespace", name)
		}
	}
	for name := range inventory.ruleProviders {
		if strings.HasPrefix(name, "open-surge-ruleset-") {
			return fmt.Errorf("imported mihomo profile rule provider %q occupies reserved open-surge-ruleset- namespace", name)
		}
	}
	for _, group := range policy.groups {
		if section, exists := inventory.targets[group.Name]; exists {
			return fmt.Errorf("generated device policy group %q conflicts with imported %s", group.Name, section)
		}
	}
	for _, provider := range policy.providers {
		if inventory.ruleProviders[provider.Name] {
			return fmt.Errorf("generated device policy rule provider %q conflicts with imported rule provider", provider.Name)
		}
	}
	for _, target := range append(append([]string(nil), policy.bundle.Compiled.SelectorTargets...), policy.bundle.Compiled.ActionTargets...) {
		if builtinPolicyTarget(target) {
			continue
		}
		if _, exists := inventory.targets[target]; !exists {
			return fmt.Errorf("device policy references unknown imported proxy or group %q", target)
		}
	}
	return nil
}

func validateManagedPolicySections(cfg config.Config, policy policySections) error {
	if policy.bundle == nil {
		return nil
	}
	available := map[string]bool{}
	if cfg.UpstreamProxy.Enabled {
		available[cfg.UpstreamProxy.Name] = true
		available["open-surge-egress"] = true
	}
	for _, target := range append(append([]string(nil), policy.bundle.Compiled.SelectorTargets...), policy.bundle.Compiled.ActionTargets...) {
		if builtinPolicyTarget(target) || available[target] {
			continue
		}
		return fmt.Errorf("device policy references unknown managed proxy or group %q", target)
	}
	return nil
}

func builtinPolicyTarget(target string) bool {
	switch strings.ToUpper(target) {
	case "DIRECT", "REJECT", "REJECT-DROP", "REJECT-TINYGIF":
		return true
	default:
		return false
	}
}

func renderSelectorGroups(groups []device.SelectorGroup) string {
	return "proxy-groups:\n" + renderSelectorGroupItems(groups)
}

func renderSelectorGroupItems(groups []device.SelectorGroup) string {
	var out strings.Builder
	for _, group := range groups {
		out.WriteString("  - name: " + group.Name + "\n")
		out.WriteString("    type: select\n")
		out.WriteString("    proxies:\n")
		for _, policy := range group.Policies {
			out.WriteString("      - " + yamlQuote(policy) + "\n")
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

func renderRuleProviders(providers []device.RuleProvider) string {
	return "rule-providers:\n" + renderRuleProviderItems(providers)
}

func renderRuleProviderItems(providers []device.RuleProvider) string {
	var out strings.Builder
	for _, provider := range providers {
		out.WriteString("  " + provider.Name + ":\n")
		out.WriteString("    type: " + provider.Type + "\n")
		out.WriteString("    behavior: " + provider.Behavior + "\n")
		if provider.Type == "http" {
			out.WriteString("    url: " + yamlQuote(provider.URL) + "\n")
			out.WriteString("    format: " + provider.Format + "\n")
			if provider.Interval > 0 {
				out.WriteString(fmt.Sprintf("    interval: %d\n", provider.Interval))
			}
		} else {
			out.WriteString("    payload:\n")
			for _, value := range provider.Payload {
				out.WriteString("      - " + yamlQuote(value) + "\n")
			}
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

func writeRuleLines(out *strings.Builder, rules []string) {
	for _, rule := range rules {
		out.WriteString("  - ")
		out.WriteString(rule)
		out.WriteString("\n")
	}
}

func yamlQuote(value string) string {
	return strconv.Quote(value)
}
