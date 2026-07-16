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
		return composeImportedPolicySections(imported.blocks, sections)
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

func composeImportedPolicySections(blocks []importedProfileBlock, policy policySections) (string, error) {
	byKey := map[string]string{}
	for _, block := range blocks {
		if _, exists := byKey[block.key]; exists {
			return "", fmt.Errorf("imported mihomo profile contains duplicate top-level %s section", block.key)
		}
		byKey[block.key] = strings.TrimRight(block.text, "\n")
	}
	if len(policy.groups) > 0 {
		byKey["proxy-groups"] = appendYAMLBlock(byKey["proxy-groups"], "proxy-groups:", renderSelectorGroupItems(policy.groups))
	}
	if len(policy.providers) > 0 {
		byKey["rule-providers"] = appendYAMLBlock(byKey["rule-providers"], "rule-providers:", renderRuleProviderItems(policy.providers))
	}
	rules, err := renderRules(byKey["rules"], orderedDevicePreRules(policy), policy.defaults)
	if err != nil {
		return "", err
	}
	byKey["rules"] = strings.TrimRight(rules, "\n")

	var out strings.Builder
	for _, key := range []string{"proxies", "proxy-providers", "proxy-groups", "rule-providers", "rules"} {
		if block := strings.TrimSpace(byKey[key]); block != "" {
			out.WriteString(block)
			out.WriteString("\n\n")
		}
	}
	return strings.TrimRight(out.String(), "\n") + "\n", nil
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

func appendYAMLBlock(existing, header, items string) string {
	if strings.TrimSpace(items) == "" {
		return strings.TrimRight(existing, "\n")
	}
	if strings.TrimSpace(existing) == "" {
		return header + "\n" + strings.TrimRight(items, "\n")
	}
	items = reindentYAMLBlockItems(items, importedYAMLBlockItemIndent(existing))
	return strings.TrimRight(existing, "\n") + "\n" + strings.TrimRight(items, "\n")
}

// Imported sections are preserved as source text, including their indentation.
// Generated items therefore need to match the section's existing top-level item
// indentation instead of assuming the two spaces used by OpenSurge fixtures.
func importedYAMLBlockItemIndent(block string) int {
	lines := strings.Split(strings.TrimRight(block, "\n"), "\n")
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent >= 2 {
			return indent
		}
	}
	return 2
}

func reindentYAMLBlockItems(items string, indent int) string {
	const generatedIndent = 2
	if indent <= generatedIndent {
		return items
	}
	prefix := strings.Repeat(" ", indent-generatedIndent)
	lines := strings.Split(items, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
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

// renderRules inserts system, device override, and dedicated-egress rules
// before global rules. Legacy device defaults remain after global rules but
// before an imported terminal MATCH so old policy documents keep their exact
// fallback behavior until the operator selects an explicit mode.
func renderRules(existing string, preRules, defaultRules []string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		return renderRules("rules:\n", append(preRules, defaultRules...), []string{"MATCH,DIRECT"})
	}
	lines := strings.Split(strings.TrimRight(existing, "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "rules:" {
		return "", fmt.Errorf("imported mihomo profile rules section is malformed")
	}
	body := lines[1:]
	terminalIndex := -1
	for i, line := range body {
		if isTerminalMatch(line) {
			if terminalIndex >= 0 {
				return "", fmt.Errorf("imported mihomo profile rules section has multiple MATCH rules")
			}
			terminalIndex = i
		}
	}
	var before, terminal []string
	if terminalIndex >= 0 {
		for _, line := range body[terminalIndex+1:] {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				return "", fmt.Errorf("imported mihomo profile MATCH rule must be terminal")
			}
		}
		before = body[:terminalIndex]
		terminal = body[terminalIndex:]
	} else {
		before = body
	}

	var out strings.Builder
	out.WriteString("rules:\n")
	ruleIndent := importedYAMLBlockItemIndent(existing)
	writeRuleLinesWithIndent(&out, preRules, ruleIndent)
	for _, line := range before {
		out.WriteString(line)
		out.WriteString("\n")
	}
	writeRuleLinesWithIndent(&out, defaultRules, ruleIndent)
	for _, line := range terminal {
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String(), nil
}

func writeRuleLines(out *strings.Builder, rules []string) {
	writeRuleLinesWithIndent(out, rules, 2)
}

func writeRuleLinesWithIndent(out *strings.Builder, rules []string, indent int) {
	prefix := strings.Repeat(" ", indent) + "- "
	for _, rule := range rules {
		out.WriteString(prefix)
		out.WriteString(rule)
		out.WriteString("\n")
	}
}

func isTerminalMatch(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "-") {
		return false
	}
	value := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
	var decoded string
	if err := yaml.Unmarshal([]byte(value), &decoded); err == nil {
		value = decoded
	}
	value = strings.ToUpper(value)
	return strings.HasPrefix(value, "MATCH,") || value == "MATCH"
}

func yamlQuote(value string) string {
	return strconv.Quote(value)
}
