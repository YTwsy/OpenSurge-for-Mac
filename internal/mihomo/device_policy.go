package mihomo

import (
	"fmt"
	"strconv"
	"strings"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
)

type policySections struct {
	groups    []device.SelectorGroup
	providers []device.RuleProvider
	preRules  []string
	defaults  []string
}

func renderPolicySections(cfg config.Config) (string, error) {
	sections, err := loadPolicySections(cfg.DevicePolicy.File)
	if err != nil {
		return "", err
	}
	if cfg.Mihomo.ProfileMode == config.MihomoProfileModeImported {
		blocks, err := loadImportedProfileBlocks(cfg.Mihomo.Profile)
		if err != nil {
			return "", err
		}
		return composeImportedPolicySections(blocks, sections)
	}
	return composeManagedPolicySections(cfg, sections), nil
}

func loadPolicySections(path string) (policySections, error) {
	if strings.TrimSpace(path) == "" {
		return policySections{}, nil
	}
	set, err := device.LoadPolicySet(path)
	if err != nil {
		return policySections{}, err
	}
	compiled, err := device.CompilePolicySet(set)
	if err != nil {
		return policySections{}, err
	}
	return policySections{
		groups:    compiled.SelectorGroups,
		providers: compiled.RuleProviders,
		preRules:  compiled.OverrideRules,
		defaults:  compiled.DefaultRules,
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

	rules := append([]string(nil), policy.preRules...)
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
	rules, err := renderRules(byKey["rules"], policy.preRules, policy.defaults)
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

func appendYAMLBlock(existing, header, items string) string {
	if strings.TrimSpace(items) == "" {
		return strings.TrimRight(existing, "\n")
	}
	if strings.TrimSpace(existing) == "" {
		return header + "\n" + strings.TrimRight(items, "\n")
	}
	return strings.TrimRight(existing, "\n") + "\n" + strings.TrimRight(items, "\n")
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

// renderRules inserts device overrides before global rules and inserts each
// device's default selector after global rules but before an imported terminal
// MATCH. This preserves imported-profile fallback semantics.
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
	writeRuleLines(&out, preRules)
	for _, line := range before {
		out.WriteString(line)
		out.WriteString("\n")
	}
	writeRuleLines(&out, defaultRules)
	for _, line := range terminal {
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String(), nil
}

func writeRuleLines(out *strings.Builder, rules []string) {
	for _, rule := range rules {
		out.WriteString("  - ")
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
	value = strings.ToUpper(value)
	return strings.HasPrefix(value, "MATCH,") || value == "MATCH"
}

func yamlQuote(value string) string {
	return strconv.Quote(value)
}
