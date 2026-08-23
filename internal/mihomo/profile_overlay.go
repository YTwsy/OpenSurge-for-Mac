package mihomo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const ProfileOverlaySchemaVersion = 1

var profileOverlayDNSFields = map[string]bool{
	"cache-algorithm":                 true,
	"default-nameserver":              true,
	"direct-nameserver":               true,
	"direct-nameserver-follow-policy": true,
	"fake-ip-filter":                  true,
	"fake-ip-filter-mode":             true,
	"fallback":                        true,
	"fallback-filter":                 true,
	"nameserver":                      true,
	"nameserver-policy":               true,
	"prefer-h3":                       true,
	"proxy-server-nameserver":         true,
	"respect-rules":                   true,
	"use-hosts":                       true,
	"use-system-hosts":                true,
}

// ProfileOverlayDocument is a deliberately small, declarative patch language
// for imported mihomo profiles. It does not expose arbitrary top-level mihomo
// configuration: OpenSurge continues to own the gateway-facing config.
type ProfileOverlayDocument struct {
	SchemaVersion  int                       `json:"schema_version" yaml:"schema-version"`
	Enabled        bool                      `json:"enabled" yaml:"enabled"`
	Rules          ProfileOverlayRuleOps     `json:"rules" yaml:"rules"`
	Proxies        ProfileOverlaySequenceOps `json:"proxies" yaml:"proxies"`
	ProxyProviders ProfileOverlayMappingOps  `json:"proxy_providers" yaml:"proxy-providers"`
	ProxyGroups    ProfileOverlayGroupOps    `json:"proxy_groups" yaml:"proxy-groups"`
	RuleProviders  ProfileOverlayMappingOps  `json:"rule_providers" yaml:"rule-providers"`
	DNS            ProfileOverlayDNSOps      `json:"dns" yaml:"dns"`
}

type ProfileOverlayRuleOps struct {
	Prepend           []string `json:"prepend" yaml:"prepend"`
	AppendBeforeMatch []string `json:"append_before_match" yaml:"append-before-match"`
}

type ProfileOverlaySequenceOps struct {
	Add     []map[string]any `json:"add" yaml:"add"`
	Replace []map[string]any `json:"replace" yaml:"replace"`
}

type ProfileOverlayMappingOps struct {
	Add     map[string]map[string]any `json:"add" yaml:"add"`
	Replace map[string]map[string]any `json:"replace" yaml:"replace"`
}

type ProfileOverlayGroupOps struct {
	Add     []map[string]any           `json:"add" yaml:"add"`
	Replace []map[string]any           `json:"replace" yaml:"replace"`
	Patch   []ProfileOverlayGroupPatch `json:"patch" yaml:"patch"`
}

type ProfileOverlayGroupPatch struct {
	Name          string   `json:"name" yaml:"name"`
	AppendProxies []string `json:"append_proxies" yaml:"append-proxies"`
	AppendUse     []string `json:"append_use" yaml:"append-use"`
}

type ProfileOverlayDNSOps struct {
	Merge  map[string]any   `json:"merge" yaml:"merge"`
	Append map[string][]any `json:"append" yaml:"append"`
}

type ProfileOverlayComposition struct {
	ProfileYAML string
	Inspection  ImportedProfileInspection
	Digest      string
}

func DefaultProfileOverlayDocument() ProfileOverlayDocument {
	return normalizeProfileOverlayDocument(ProfileOverlayDocument{SchemaVersion: ProfileOverlaySchemaVersion})
}

func ParseProfileOverlay(data []byte) (ProfileOverlayDocument, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return ProfileOverlayDocument{}, fmt.Errorf("profile overlay is empty")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var document ProfileOverlayDocument
	if err := decoder.Decode(&document); err != nil {
		return ProfileOverlayDocument{}, fmt.Errorf("parse profile overlay: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ProfileOverlayDocument{}, fmt.Errorf("profile overlay must contain exactly one YAML document")
		}
		return ProfileOverlayDocument{}, fmt.Errorf("parse profile overlay: %w", err)
	}
	document = normalizeProfileOverlayDocument(document)
	if err := ValidateProfileOverlay(document); err != nil {
		return ProfileOverlayDocument{}, err
	}
	return document, nil
}

func RenderProfileOverlay(document ProfileOverlayDocument) ([]byte, error) {
	document = normalizeProfileOverlayDocument(document)
	if err := ValidateProfileOverlay(document); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, fmt.Errorf("render profile overlay: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("render profile overlay: %w", err)
	}
	return out.Bytes(), nil
}

func ProfileOverlayDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func ValidateProfileOverlay(document ProfileOverlayDocument) error {
	if document.SchemaVersion != ProfileOverlaySchemaVersion {
		return fmt.Errorf("profile overlay schema-version must be %d", ProfileOverlaySchemaVersion)
	}
	for _, entry := range append(append([]string{}, document.Rules.Prepend...), document.Rules.AppendBeforeMatch...) {
		if strings.TrimSpace(entry) == "" {
			return fmt.Errorf("profile overlay rules must not contain empty entries")
		}
		if isTerminalMatchValue(entry) {
			return fmt.Errorf("profile overlay rules must not add MATCH; OpenSurge preserves the imported terminal MATCH")
		}
	}
	if err := validateOverlaySequenceOps("proxies", document.Proxies.Add, document.Proxies.Replace); err != nil {
		return err
	}
	if err := validateOverlaySequenceOps("proxy-groups", document.ProxyGroups.Add, document.ProxyGroups.Replace); err != nil {
		return err
	}
	if err := validateOverlayMappingOps("proxy-providers", document.ProxyProviders); err != nil {
		return err
	}
	if err := validateOverlayMappingOps("rule-providers", document.RuleProviders); err != nil {
		return err
	}
	patchNames := map[string]bool{}
	for _, patch := range document.ProxyGroups.Patch {
		if strings.TrimSpace(patch.Name) == "" {
			return fmt.Errorf("proxy-groups.patch entry requires name")
		}
		if strings.TrimSpace(patch.Name) != patch.Name {
			return fmt.Errorf("proxy-groups.patch target name must be a trimmed string")
		}
		if err := validateOverlayTargetName("proxy-groups.patch", patch.Name); err != nil {
			return err
		}
		if patchNames[patch.Name] {
			return fmt.Errorf("proxy-groups.patch contains duplicate group %q", patch.Name)
		}
		patchNames[patch.Name] = true
		if len(patch.AppendProxies) == 0 && len(patch.AppendUse) == 0 {
			return fmt.Errorf("proxy-groups.patch %q must append proxies or providers", patch.Name)
		}
		if err := validateOverlayStringList("proxy-groups.patch "+patch.Name+" append-proxies", patch.AppendProxies); err != nil {
			return err
		}
		if err := validateOverlayStringList("proxy-groups.patch "+patch.Name+" append-use", patch.AppendUse); err != nil {
			return err
		}
	}
	for field := range document.DNS.Merge {
		if gatewayOwnedDNSFields[field] {
			return fmt.Errorf("dns.%s is managed by OpenSurge and cannot be changed by a profile overlay", field)
		}
		if !profileOverlayDNSFields[field] {
			return fmt.Errorf("dns.%s is not a supported resolver or filtering field for a profile overlay", field)
		}
	}
	for field, values := range document.DNS.Append {
		if gatewayOwnedDNSFields[field] {
			return fmt.Errorf("dns.%s is managed by OpenSurge and cannot be changed by a profile overlay", field)
		}
		if !profileOverlayDNSFields[field] {
			return fmt.Errorf("dns.%s is not a supported resolver or filtering field for a profile overlay", field)
		}
		if len(values) == 0 {
			return fmt.Errorf("dns.append.%s must contain at least one value", field)
		}
	}
	return nil
}

func ComposeProfileOverlay(source []byte, document ProfileOverlayDocument) (ProfileOverlayComposition, error) {
	document = normalizeProfileOverlayDocument(document)
	if err := ValidateProfileOverlay(document); err != nil {
		return ProfileOverlayComposition{}, err
	}
	base, err := parseImportedProfile(source)
	if err != nil {
		return ProfileOverlayComposition{}, fmt.Errorf("parse imported mihomo profile: %w", err)
	}
	if !document.Enabled {
		inspection, err := InspectImportedProfile(source)
		if err != nil {
			return ProfileOverlayComposition{}, err
		}
		return ProfileOverlayComposition{ProfileYAML: string(source), Inspection: inspection, Digest: ProfileOverlayDigest(source)}, nil
	}

	targets := make(map[string]string, len(base.inventory.targets))
	for name, section := range base.inventory.targets {
		targets[name] = section
	}
	if err := applyOverlayNamedSequence(&base, "proxies", document.Proxies, targets); err != nil {
		return ProfileOverlayComposition{}, err
	}
	if err := applyOverlayNamedMapping(&base, "proxy-providers", document.ProxyProviders, base.inventory.proxyProviders); err != nil {
		return ProfileOverlayComposition{}, err
	}
	groupOps := ProfileOverlaySequenceOps{Add: document.ProxyGroups.Add, Replace: document.ProxyGroups.Replace}
	if err := applyOverlayNamedSequence(&base, "proxy-groups", groupOps, targets); err != nil {
		return ProfileOverlayComposition{}, err
	}
	if err := applyOverlayNamedMapping(&base, "rule-providers", document.RuleProviders, base.inventory.ruleProviders); err != nil {
		return ProfileOverlayComposition{}, err
	}
	for _, patch := range document.ProxyGroups.Patch {
		if err := applyOverlayGroupPatch(&base, patch, targets); err != nil {
			return ProfileOverlayComposition{}, err
		}
	}
	for _, operations := range [][]map[string]any{document.ProxyGroups.Add, document.ProxyGroups.Replace} {
		for _, entry := range operations {
			name, _ := overlayMapString(entry, "name")
			if err := validateOverlayGroupReferences(base.sections["proxy-groups"], name, targets, base.inventory.proxyProviders); err != nil {
				return ProfileOverlayComposition{}, err
			}
		}
	}
	if err := validateOverlayRuleReferences(document.Rules.Prepend, targets, base.inventory.ruleProviders); err != nil {
		return ProfileOverlayComposition{}, err
	}
	if err := validateOverlayRuleReferences(document.Rules.AppendBeforeMatch, targets, base.inventory.ruleProviders); err != nil {
		return ProfileOverlayComposition{}, err
	}
	if len(document.Rules.AppendBeforeMatch) > 0 && !importedRulesHaveTerminalMatch(base.sections["rules"]) {
		return ProfileOverlayComposition{}, fmt.Errorf("rules.append-before-match requires the imported profile to end with MATCH")
	}
	applyOverlayRules(base.sections["rules"], document.Rules)

	dns, err := decodeSingleYAMLMapping([]byte(base.dnsResolverFields))
	if err != nil {
		return ProfileOverlayComposition{}, fmt.Errorf("parse retained imported DNS: %w", err)
	}
	if err := applyOverlayDNS(dns, document.DNS); err != nil {
		return ProfileOverlayComposition{}, err
	}
	rendered, err := renderComposedProfileSource(&base, dns)
	if err != nil {
		return ProfileOverlayComposition{}, err
	}
	inspection, err := InspectImportedProfile(rendered)
	if err != nil {
		return ProfileOverlayComposition{}, fmt.Errorf("validate composed profile: %w", err)
	}
	return ProfileOverlayComposition{ProfileYAML: string(rendered), Inspection: inspection, Digest: ProfileOverlayDigest(rendered)}, nil
}

func normalizeProfileOverlayDocument(document ProfileOverlayDocument) ProfileOverlayDocument {
	if document.Rules.Prepend == nil {
		document.Rules.Prepend = []string{}
	}
	if document.Rules.AppendBeforeMatch == nil {
		document.Rules.AppendBeforeMatch = []string{}
	}
	if document.Proxies.Add == nil {
		document.Proxies.Add = []map[string]any{}
	}
	if document.Proxies.Replace == nil {
		document.Proxies.Replace = []map[string]any{}
	}
	if document.ProxyProviders.Add == nil {
		document.ProxyProviders.Add = map[string]map[string]any{}
	}
	if document.ProxyProviders.Replace == nil {
		document.ProxyProviders.Replace = map[string]map[string]any{}
	}
	if document.ProxyGroups.Add == nil {
		document.ProxyGroups.Add = []map[string]any{}
	}
	if document.ProxyGroups.Replace == nil {
		document.ProxyGroups.Replace = []map[string]any{}
	}
	if document.ProxyGroups.Patch == nil {
		document.ProxyGroups.Patch = []ProfileOverlayGroupPatch{}
	}
	if document.RuleProviders.Add == nil {
		document.RuleProviders.Add = map[string]map[string]any{}
	}
	if document.RuleProviders.Replace == nil {
		document.RuleProviders.Replace = map[string]map[string]any{}
	}
	if document.DNS.Merge == nil {
		document.DNS.Merge = map[string]any{}
	}
	if document.DNS.Append == nil {
		document.DNS.Append = map[string][]any{}
	}
	return document
}

func validateOverlaySequenceOps(section string, add, replace []map[string]any) error {
	seen := map[string]string{}
	for operation, entries := range map[string][]map[string]any{"add": add, "replace": replace} {
		for _, entry := range entries {
			name, ok := overlayMapString(entry, "name")
			if !ok || strings.TrimSpace(name) == "" {
				return fmt.Errorf("%s.%s entry requires a non-empty string name", section, operation)
			}
			if err := validateOverlayTargetName(section, name); err != nil {
				return err
			}
			if prior := seen[name]; prior != "" {
				return fmt.Errorf("%s target %q appears in both %s and %s operations", section, name, prior, operation)
			}
			seen[name] = operation
		}
	}
	return nil
}

func validateOverlayMappingOps(section string, operations ProfileOverlayMappingOps) error {
	for name := range operations.Add {
		if err := validateOverlayTargetName(section, name); err != nil {
			return err
		}
		if _, exists := operations.Replace[name]; exists {
			return fmt.Errorf("%s target %q appears in both add and replace operations", section, name)
		}
	}
	for name := range operations.Replace {
		if err := validateOverlayTargetName(section, name); err != nil {
			return err
		}
	}
	return nil
}

func validateOverlayTargetName(section, name string) error {
	if strings.TrimSpace(name) != name || name == "" {
		return fmt.Errorf("%s target name must be a non-empty trimmed string", section)
	}
	if IsLocalRoutingGroup(name) || strings.HasPrefix(name, "device/") || strings.HasPrefix(name, "open-surge-ruleset-") {
		return fmt.Errorf("%s target %q uses a reserved OpenSurge namespace", section, name)
	}
	return nil
}

func validateOverlayStringList(label string, values []string) error {
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) != value || value == "" {
			return fmt.Errorf("%s values must be non-empty trimmed strings", label)
		}
		if seen[value] {
			return fmt.Errorf("%s contains duplicate value %q", label, value)
		}
		seen[value] = true
	}
	return nil
}

func applyOverlayNamedSequence(profile *importedProfile, section string, operations ProfileOverlaySequenceOps, targets map[string]string) error {
	node := ensureImportedSection(profile, section, yaml.SequenceNode, "!!seq")
	for _, entry := range operations.Replace {
		name, _ := overlayMapString(entry, "name")
		if existingSection := targets[name]; existingSection != section {
			if existingSection == "" {
				return fmt.Errorf("%s.replace target %q does not exist in the imported profile", section, name)
			}
			return fmt.Errorf("%s.replace target %q is an imported %s", section, name, existingSection)
		}
		replacement, err := overlayMappingNode(entry)
		if err != nil {
			return fmt.Errorf("%s.replace %q: %w", section, name, err)
		}
		index := namedSequenceIndex(node, name)
		if index < 0 {
			return fmt.Errorf("%s.replace target %q does not exist in the imported profile", section, name)
		}
		node.Content[index] = replacement
	}
	for _, entry := range operations.Add {
		name, _ := overlayMapString(entry, "name")
		if existingSection := targets[name]; existingSection != "" {
			return fmt.Errorf("%s.add target %q conflicts with imported %s", section, name, existingSection)
		}
		addition, err := overlayMappingNode(entry)
		if err != nil {
			return fmt.Errorf("%s.add %q: %w", section, name, err)
		}
		node.Content = append(node.Content, addition)
		targets[name] = section
	}
	return nil
}

func applyOverlayNamedMapping(profile *importedProfile, section string, operations ProfileOverlayMappingOps, existing map[string]bool) error {
	node := ensureImportedSection(profile, section, yaml.MappingNode, "!!map")
	for _, name := range sortedOverlayMapKeys(operations.Replace) {
		if !existing[name] {
			return fmt.Errorf("%s.replace target %q does not exist in the imported profile", section, name)
		}
		replacement, err := overlayMappingNode(operations.Replace[name])
		if err != nil {
			return fmt.Errorf("%s.replace %q: %w", section, name, err)
		}
		index := mappingValueIndex(node, name)
		if index < 0 {
			return fmt.Errorf("%s.replace target %q does not exist in the imported profile", section, name)
		}
		node.Content[index] = replacement
	}
	for _, name := range sortedOverlayMapKeys(operations.Add) {
		if existing[name] {
			return fmt.Errorf("%s.add target %q already exists in the imported profile", section, name)
		}
		addition, err := overlayMappingNode(operations.Add[name])
		if err != nil {
			return fmt.Errorf("%s.add %q: %w", section, name, err)
		}
		node.Content = append(node.Content, stringNode(name), addition)
		existing[name] = true
	}
	return nil
}

func applyOverlayGroupPatch(profile *importedProfile, patch ProfileOverlayGroupPatch, targets map[string]string) error {
	if section := targets[patch.Name]; section != "proxy-groups" {
		if section == "" {
			return fmt.Errorf("proxy-groups.patch target %q does not exist", patch.Name)
		}
		return fmt.Errorf("proxy-groups.patch target %q is an imported %s", patch.Name, section)
	}
	groups := profile.sections["proxy-groups"]
	index := namedSequenceIndex(groups, patch.Name)
	if index < 0 {
		return fmt.Errorf("proxy-groups.patch target %q does not exist", patch.Name)
	}
	group := resolveAlias(groups.Content[index])
	if group == nil || group.Kind != yaml.MappingNode {
		return fmt.Errorf("proxy-groups.patch target %q is not a mapping", patch.Name)
	}
	if err := appendUniqueMappingSequence(group, "proxies", patch.AppendProxies); err != nil {
		return fmt.Errorf("proxy-groups.patch %q: %w", patch.Name, err)
	}
	if err := appendUniqueMappingSequence(group, "use", patch.AppendUse); err != nil {
		return fmt.Errorf("proxy-groups.patch %q: %w", patch.Name, err)
	}
	for _, candidate := range patch.AppendProxies {
		if !overlayBuiltinTarget(candidate) && targets[candidate] == "" {
			return fmt.Errorf("proxy-groups.patch %q references unknown proxy or group %q", patch.Name, candidate)
		}
	}
	for _, provider := range patch.AppendUse {
		if !profile.inventory.proxyProviders[provider] {
			return fmt.Errorf("proxy-groups.patch %q references unknown proxy provider %q", patch.Name, provider)
		}
	}
	return nil
}

func validateOverlayGroupReferences(groups *yaml.Node, name string, targets map[string]string, providers map[string]bool) error {
	index := namedSequenceIndex(groups, name)
	if index < 0 {
		return fmt.Errorf("proxy-groups target %q does not exist after composition", name)
	}
	group := resolveAlias(groups.Content[index])
	for _, field := range []struct {
		name   string
		label  string
		exists func(string) bool
	}{
		{name: "proxies", label: "proxy or group", exists: func(value string) bool { return overlayBuiltinTarget(value) || targets[value] != "" }},
		{name: "use", label: "proxy provider", exists: func(value string) bool { return providers[value] }},
	} {
		valueIndex := mappingValueIndex(group, field.name)
		if valueIndex < 0 {
			continue
		}
		sequence := resolveAlias(group.Content[valueIndex])
		if sequence == nil || sequence.Kind != yaml.SequenceNode {
			return fmt.Errorf("proxy-groups target %q field %s must be a sequence", name, field.name)
		}
		for _, item := range sequence.Content {
			candidate, ok := scalarStringValue(item)
			if !ok || strings.TrimSpace(candidate) == "" {
				return fmt.Errorf("proxy-groups target %q field %s must contain non-empty strings", name, field.name)
			}
			if !field.exists(candidate) {
				return fmt.Errorf("proxy-groups target %q references unknown %s %q", name, field.label, candidate)
			}
		}
	}
	return nil
}

func validateOverlayRuleReferences(rules []string, targets map[string]string, providers map[string]bool) error {
	for _, rule := range rules {
		parts := strings.Split(rule, ",")
		if len(parts) < 2 {
			return fmt.Errorf("profile overlay rule %q is incomplete", rule)
		}
		if strings.EqualFold(strings.TrimSpace(parts[0]), "RULE-SET") {
			provider := strings.TrimSpace(parts[1])
			if !providers[provider] {
				return fmt.Errorf("profile overlay rule %q references unknown rule provider %q", rule, provider)
			}
		}
		target := importedRuleTarget(rule)
		if target == "" {
			return fmt.Errorf("profile overlay rule %q does not have a policy target", rule)
		}
		if !overlayBuiltinTarget(target) && targets[target] == "" {
			return fmt.Errorf("profile overlay rule %q references unknown proxy or group %q", rule, target)
		}
	}
	return nil
}

func applyOverlayRules(rules *yaml.Node, operations ProfileOverlayRuleOps) {
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
	content := make([]*yaml.Node, 0, len(operations.Prepend)+len(before)+len(operations.AppendBeforeMatch)+len(terminal))
	content = append(content, ruleNodes(operations.Prepend)...)
	content = append(content, before...)
	content = append(content, ruleNodes(operations.AppendBeforeMatch)...)
	content = append(content, terminal...)
	rules.Content = content
	if len(operations.Prepend) > 0 || len(operations.AppendBeforeMatch) > 0 {
		rules.Style &^= yaml.FlowStyle
	}
}

func importedRulesHaveTerminalMatch(rules *yaml.Node) bool {
	if rules == nil || len(rules.Content) == 0 {
		return false
	}
	value, ok := scalarStringValue(rules.Content[len(rules.Content)-1])
	return ok && isTerminalMatchValue(value)
}

func applyOverlayDNS(dns *yaml.Node, operations ProfileOverlayDNSOps) error {
	patch, err := overlayMappingNode(operations.Merge)
	if err != nil {
		return fmt.Errorf("dns.merge: %w", err)
	}
	deepMergeYAMLMapping(dns, patch)
	for _, field := range sortedOverlaySliceMapKeys(operations.Append) {
		index := mappingValueIndex(dns, field)
		var target *yaml.Node
		if index < 0 {
			target = sequenceNode()
			dns.Content = append(dns.Content, stringNode(field), target)
		} else {
			target = resolveAlias(dns.Content[index])
			if target == nil || target.Kind != yaml.SequenceNode {
				return fmt.Errorf("dns.append.%s requires the existing DNS field to be a sequence", field)
			}
		}
		for _, value := range operations.Append[field] {
			node, err := overlayValueNode(value)
			if err != nil {
				return fmt.Errorf("dns.append.%s: %w", field, err)
			}
			target.Content = append(target.Content, node)
		}
	}
	return nil
}

func renderComposedProfileSource(profile *importedProfile, dns *yaml.Node) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, section := range profile.sectionOrder {
		value := profile.sections[section]
		if value == nil {
			continue
		}
		key := profile.sectionKeys[section]
		if key == nil {
			key = stringNode(section)
		}
		root.Content = append(root.Content, key, value)
	}
	if dns != nil && len(dns.Content) > 0 {
		root.Content = append(root.Content, stringNode("dns"), dns)
	}
	rendered, err := encodeYAMLNode(root)
	return []byte(rendered), err
}

func deepMergeYAMLMapping(target, patch *yaml.Node) {
	if target == nil || patch == nil || target.Kind != yaml.MappingNode || patch.Kind != yaml.MappingNode {
		return
	}
	for index := 0; index < len(patch.Content); index += 2 {
		key, value := patch.Content[index], patch.Content[index+1]
		targetIndex := mappingValueIndex(target, key.Value)
		if targetIndex < 0 {
			target.Content = append(target.Content, key, value)
			continue
		}
		existing := resolveAlias(target.Content[targetIndex])
		candidate := resolveAlias(value)
		if existing != nil && candidate != nil && existing.Kind == yaml.MappingNode && candidate.Kind == yaml.MappingNode {
			deepMergeYAMLMapping(existing, candidate)
			continue
		}
		target.Content[targetIndex] = value
	}
}

func appendUniqueMappingSequence(mapping *yaml.Node, field string, values []string) error {
	if len(values) == 0 {
		return nil
	}
	index := mappingValueIndex(mapping, field)
	var sequence *yaml.Node
	if index < 0 {
		sequence = sequenceNode()
		mapping.Content = append(mapping.Content, stringNode(field), sequence)
	} else {
		sequence = resolveAlias(mapping.Content[index])
		if sequence == nil || sequence.Kind != yaml.SequenceNode {
			return fmt.Errorf("%s must be a sequence", field)
		}
	}
	existing := map[string]bool{}
	for _, item := range sequence.Content {
		if value, ok := scalarStringValue(item); ok {
			existing[value] = true
		}
	}
	for _, value := range values {
		if existing[value] {
			return fmt.Errorf("%s already contains %q", field, value)
		}
		sequence.Content = append(sequence.Content, quotedStringNode(value))
		existing[value] = true
	}
	return nil
}

func namedSequenceIndex(sequence *yaml.Node, name string) int {
	if sequence == nil || sequence.Kind != yaml.SequenceNode {
		return -1
	}
	for index, item := range sequence.Content {
		if value, ok := mappingScalar(resolveAlias(item), "name"); ok && value == name {
			return index
		}
	}
	return -1
}

func mappingValueIndex(mapping *yaml.Node, name string) int {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return -1
	}
	for index := 0; index < len(mapping.Content); index += 2 {
		if key := mapping.Content[index]; isStringScalar(key) && key.Value == name {
			return index + 1
		}
	}
	return -1
}

func overlayMapString(mapping map[string]any, key string) (string, bool) {
	value, exists := mapping[key]
	if !exists {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func overlayMappingNode(value map[string]any) (*yaml.Node, error) {
	node, err := overlayValueNode(value)
	if err != nil {
		return nil, err
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("value must be a mapping")
	}
	return node, nil
}

func overlayValueNode(value any) (*yaml.Node, error) {
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return nil, err
	}
	return &node, nil
}

func sortedOverlayMapKeys(values map[string]map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedOverlaySliceMapKeys(values map[string][]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func overlayBuiltinTarget(value string) bool {
	switch strings.ToUpper(value) {
	case "DIRECT", "REJECT", "REJECT-DROP", "REJECT-TINYGIF", "PASS", "COMPATIBLE", "GLOBAL":
		return true
	default:
		return false
	}
}
