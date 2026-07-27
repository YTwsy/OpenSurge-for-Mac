package mihomo

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var importableProfileSectionOrder = []string{
	"proxies",
	"proxy-providers",
	"proxy-groups",
	"rule-providers",
	"rules",
}

const defaultDNSResolverFieldsYAML = `nameserver:
  - 1.1.1.1
  - 8.8.8.8
`

// These fields define the gateway-facing DNS contract and must not be
// overridden by a desktop profile. The remaining dns fields describe resolver
// and filtering behavior; preserving them is required for profiles whose proxy
// server hostnames depend on nameserver-policy or private resolvers.
var gatewayOwnedDNSFields = map[string]bool{
	"enable":        true,
	"listen":        true,
	"ipv6":          true,
	"enhanced-mode": true,
	"fake-ip-range": true,
}

// ImportedProfileInspection is the structural inventory shared by the source
// preview and the authoritative imported-profile renderer.
type ImportedProfileInspection struct {
	Proxies        []string
	ProxyProviders []string
	ProxyGroups    []string
	RuleProviders  []string
	RuleCount      int
	TerminalMatch  bool
	Warnings       []string
}

type importedProfile struct {
	sections          map[string]*yaml.Node
	sectionKeys       map[string]*yaml.Node
	sectionOrder      []string
	inventory         importedProfileInventory
	dnsResolverFields string
}

type importedProfileInventory struct {
	targets            map[string]string
	proxies            []string
	proxyProviderNames []string
	proxyGroups        []string
	ruleProviderNames  []string
	proxyProviders     map[string]bool
	ruleProviders      map[string]bool
}

func LoadImportedProfileSections(path string) (string, error) {
	profile, err := loadImportedProfile(path)
	if err != nil {
		return "", err
	}
	return renderImportedProfileSections(&profile)
}

func InspectImportedProfile(data []byte) (ImportedProfileInspection, error) {
	profile, err := parseImportedProfile(data)
	if err != nil {
		return ImportedProfileInspection{}, err
	}
	rules := profile.sections["rules"]
	inspection := ImportedProfileInspection{
		Proxies:        profile.inventory.proxies,
		ProxyProviders: profile.inventory.proxyProviderNames,
		ProxyGroups:    profile.inventory.proxyGroups,
		RuleProviders:  profile.inventory.ruleProviderNames,
		RuleCount:      len(rules.Content),
		Warnings:       []string{},
	}
	if len(rules.Content) > 0 {
		if value, ok := scalarStringValue(rules.Content[len(rules.Content)-1]); ok {
			inspection.TerminalMatch = isTerminalMatchValue(value)
		}
	}
	if !inspection.TerminalMatch {
		inspection.Warnings = append(inspection.Warnings, "rules do not end in terminal MATCH")
	}
	return inspection, nil
}

func loadImportedProfile(path string) (importedProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return importedProfile{}, fmt.Errorf("read imported mihomo profile: %w", err)
	}
	profile, err := parseImportedProfile(data)
	if err != nil {
		return importedProfile{}, fmt.Errorf("parse imported mihomo profile: %w", err)
	}
	for _, section := range []string{"proxy-providers", "rule-providers"} {
		if err := rewriteProviderPaths(profile.sections[section], filepath.Dir(path)); err != nil {
			return importedProfile{}, fmt.Errorf("rewrite imported mihomo profile %s: %w", section, err)
		}
	}
	return profile, nil
}

func parseImportedProfile(data []byte) (importedProfile, error) {
	root, err := decodeSingleYAMLMapping(data)
	if err != nil {
		return importedProfile{}, err
	}

	allSections := make(map[string]*yaml.Node, len(root.Content)/2)
	allSectionKeys := make(map[string]*yaml.Node, len(root.Content)/2)
	sectionOrder := make([]string, 0, len(importableProfileSectionOrder))
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if !isStringScalar(key) || strings.TrimSpace(key.Value) == "" {
			return importedProfile{}, fmt.Errorf("top-level key must be a non-empty string")
		}
		if _, exists := allSections[key.Value]; exists {
			return importedProfile{}, fmt.Errorf("duplicate top-level section %q", key.Value)
		}
		allSections[key.Value] = value
		allSectionKeys[key.Value] = key
		if isImportableProfileSection(key.Value) {
			sectionOrder = append(sectionOrder, key.Value)
		}
	}
	if err := validateMappingKeys(root); err != nil {
		return importedProfile{}, err
	}

	sections := make(map[string]*yaml.Node, len(importableProfileSectionOrder))
	sectionKeys := make(map[string]*yaml.Node, len(importableProfileSectionOrder))
	for _, section := range importableProfileSectionOrder {
		if value := allSections[section]; value != nil {
			sections[section] = value
			sectionKeys[section] = allSectionKeys[section]
		}
	}
	if err := validateIncludedAliases(sections); err != nil {
		return importedProfile{}, err
	}

	rules := sections["rules"]
	if rules == nil {
		return importedProfile{}, fmt.Errorf("imported mihomo profile must contain a top-level rules section")
	}
	if err := validateImportedRules(rules); err != nil {
		return importedProfile{}, err
	}

	inventory := importedProfileInventory{
		targets:        map[string]string{},
		proxyProviders: map[string]bool{},
		ruleProviders:  map[string]bool{},
	}
	inventory.proxies, err = collectNamedSequence(allSections["proxies"], "proxies", inventory.targets)
	if err != nil {
		return importedProfile{}, err
	}
	inventory.proxyGroups, err = collectNamedSequence(allSections["proxy-groups"], "proxy-groups", inventory.targets)
	if err != nil {
		return importedProfile{}, err
	}
	inventory.proxyProviderNames, err = collectNamedMapping(allSections["proxy-providers"], "proxy-providers", inventory.proxyProviders)
	if err != nil {
		return importedProfile{}, err
	}
	inventory.ruleProviderNames, err = collectNamedMapping(allSections["rule-providers"], "rule-providers", inventory.ruleProviders)
	if err != nil {
		return importedProfile{}, err
	}
	for _, section := range []string{"proxy-providers", "rule-providers"} {
		if err := validateProviderPaths(allSections[section]); err != nil {
			return importedProfile{}, fmt.Errorf("%s: %w", section, err)
		}
	}

	dnsResolverFields, err := renderImportedDNSResolverFields(root)
	if err != nil {
		return importedProfile{}, fmt.Errorf("dns: %w", err)
	}
	return importedProfile{
		sections:          sections,
		sectionKeys:       sectionKeys,
		sectionOrder:      sectionOrder,
		inventory:         inventory,
		dnsResolverFields: dnsResolverFields,
	}, nil
}

func isImportableProfileSection(value string) bool {
	for _, section := range importableProfileSectionOrder {
		if value == section {
			return true
		}
	}
	return false
}

func decodeSingleYAMLMapping(data []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("profile is empty")
		}
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple YAML documents are not supported")
		}
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("top-level YAML must be a mapping")
	}
	return document.Content[0], nil
}

func validateImportedRules(rules *yaml.Node) error {
	if rules.Kind != yaml.SequenceNode {
		return fmt.Errorf("rules must be a sequence")
	}
	matchIndex := -1
	for i, rule := range rules.Content {
		value, ok := scalarStringValue(rule)
		if !ok {
			return fmt.Errorf("rules entries must be strings")
		}
		if !isTerminalMatchValue(value) {
			continue
		}
		if matchIndex >= 0 {
			return fmt.Errorf("imported mihomo profile rules section has multiple MATCH rules")
		}
		matchIndex = i
	}
	if matchIndex >= 0 && matchIndex != len(rules.Content)-1 {
		return fmt.Errorf("imported mihomo profile MATCH rule must be terminal")
	}
	return nil
}

func isTerminalMatchValue(value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	return strings.HasPrefix(value, "MATCH,") || value == "MATCH"
}

func isStringScalar(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && (node.Tag == "" || node.Tag == "!!str")
}

func scalarStringValue(node *yaml.Node) (string, bool) {
	node = resolveAlias(node)
	if !isStringScalar(node) {
		return "", false
	}
	return node.Value, true
}

func collectNamedSequence(node *yaml.Node, section string, names map[string]string) ([]string, error) {
	collected := make([]string, 0)
	if node == nil {
		return collected, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s must be a sequence", section)
	}
	for _, item := range node.Content {
		item = resolveAlias(item)
		if item == nil || item.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s entries must be mappings", section)
		}
		name, ok := mappingScalar(item, "name")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("%s entry is missing a scalar name", section)
		}
		if prior, exists := names[name]; exists {
			return nil, fmt.Errorf("duplicate imported target name %q in %s and %s", name, prior, section)
		}
		names[name] = section
		collected = append(collected, name)
	}
	return collected, nil
}

func collectNamedMapping(node *yaml.Node, section string, names map[string]bool) ([]string, error) {
	collected := make([]string, 0)
	if node == nil {
		return collected, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", section)
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], resolveAlias(node.Content[i+1])
		if !isStringScalar(key) || strings.TrimSpace(key.Value) == "" {
			return nil, fmt.Errorf("%s key must be a non-empty string", section)
		}
		if value == nil || value.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s entry %q must be a mapping", section, key.Value)
		}
		if names[key.Value] {
			return nil, fmt.Errorf("duplicate %s name %q", section, key.Value)
		}
		names[key.Value] = true
		collected = append(collected, key.Value)
	}
	return collected, nil
}

func mappingScalar(node *yaml.Node, wanted string) (string, bool) {
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if isStringScalar(key) && key.Value == wanted {
			return scalarStringValue(value)
		}
	}
	return "", false
}

func validateMappingKeys(node *yaml.Node) error {
	visited := map[*yaml.Node]bool{}
	var walk func(*yaml.Node) error
	walk = func(current *yaml.Node) error {
		if current == nil || visited[current] {
			return nil
		}
		visited[current] = true
		if current.Kind == yaml.MappingNode {
			seen := map[string]bool{}
			for i := 0; i < len(current.Content); i += 2 {
				key := current.Content[i]
				identity, err := mappingKeyIdentity(key)
				if err != nil {
					return err
				}
				if seen[identity] {
					return fmt.Errorf("duplicate mapping key %q at line %d", key.Value, key.Line)
				}
				seen[identity] = true
			}
		}
		for _, child := range current.Content {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(node)
}

func mappingKeyIdentity(node *yaml.Node) (string, error) {
	if node == nil {
		return "", fmt.Errorf("mapping key is empty")
	}
	if node.Kind == yaml.ScalarNode {
		return fmt.Sprintf("%d\x00%s\x00%s", node.Kind, node.Tag, node.Value), nil
	}
	rendered, err := encodeYAMLNode(node)
	if err != nil {
		return "", fmt.Errorf("encode mapping key: %w", err)
	}
	return fmt.Sprintf("%d\x00%s", node.Kind, rendered), nil
}

func validateIncludedAliases(sections map[string]*yaml.Node) error {
	anchors := map[string]bool{}
	aliases := map[string]bool{}
	for _, section := range importableProfileSectionOrder {
		collectYAMLAnchors(sections[section], anchors, aliases)
	}
	for alias := range aliases {
		if !anchors[alias] {
			return fmt.Errorf("imported section references YAML anchor %q outside importable sections", alias)
		}
	}
	return nil
}

func collectYAMLAnchors(node *yaml.Node, anchors, aliases map[string]bool) {
	if node == nil {
		return
	}
	if node.Anchor != "" {
		anchors[node.Anchor] = true
	}
	if node.Kind == yaml.AliasNode {
		aliases[node.Value] = true
	}
	for _, child := range node.Content {
		collectYAMLAnchors(child, anchors, aliases)
	}
}

func renderImportedDNSResolverFields(root *yaml.Node) (string, error) {
	var dns *yaml.Node
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if isStringScalar(key) && key.Value == "dns" {
			dns = value
			break
		}
	}
	if dns == nil {
		return strings.TrimRight(defaultDNSResolverFieldsYAML, "\n"), nil
	}
	if dns.Kind != yaml.MappingNode {
		return "", fmt.Errorf("dns must be a mapping")
	}

	filtered := yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	seen := make(map[string]bool, len(dns.Content)/2)
	hasNameserver := false
	for i := 0; i < len(dns.Content); i += 2 {
		key, value := dns.Content[i], dns.Content[i+1]
		if !isStringScalar(key) || strings.TrimSpace(key.Value) == "" {
			return "", fmt.Errorf("dns field name must be a non-empty string")
		}
		if seen[key.Value] {
			return "", fmt.Errorf("duplicate dns field %q", key.Value)
		}
		seen[key.Value] = true
		if gatewayOwnedDNSFields[key.Value] {
			continue
		}
		if key.Value == "nameserver" {
			hasNameserver = true
		}
		filtered.Content = append(filtered.Content, key, value)
	}

	if !hasNameserver {
		defaults, err := decodeSingleYAMLMapping([]byte(defaultDNSResolverFieldsYAML))
		if err != nil {
			return "", err
		}
		filtered.Content = append(defaults.Content[:2], filtered.Content...)
	}
	if err := validateNodeAliases(&filtered); err != nil {
		return "", fmt.Errorf("dns resolver fields: %w", err)
	}
	return encodeYAMLNode(&filtered)
}

func validateNodeAliases(node *yaml.Node) error {
	anchors := map[string]bool{}
	aliases := map[string]bool{}
	collectYAMLAnchors(node, anchors, aliases)
	for alias := range aliases {
		if !anchors[alias] {
			return fmt.Errorf("references YAML anchor %q outside the retained section", alias)
		}
	}
	return nil
}

func renderImportedProfileSections(profile *importedProfile) (string, error) {
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
	return encodeYAMLNode(root)
}

func encodeYAMLNode(node *yaml.Node) (string, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(node); err != nil {
		return "", err
	}
	if err := encoder.Close(); err != nil {
		return "", err
	}
	return strings.TrimRight(out.String(), "\n") + "\n", nil
}

func validateProviderPaths(section *yaml.Node) error {
	return visitProviderPaths(section, nil)
}

func rewriteProviderPaths(section *yaml.Node, profileDir string) error {
	return visitProviderPaths(section, func(value *yaml.Node) {
		if relativeProviderPath(value.Value) {
			value.Value = filepath.Join(profileDir, value.Value)
		}
	})
}

func visitProviderPaths(section *yaml.Node, visit func(*yaml.Node)) error {
	if section == nil {
		return nil
	}
	if section.Kind != yaml.MappingNode {
		return fmt.Errorf("must be a mapping")
	}
	for i := 1; i < len(section.Content); i += 2 {
		provider := resolveAlias(section.Content[i])
		if provider == nil || provider.Kind != yaml.MappingNode {
			continue
		}
		for fieldIndex := 0; fieldIndex < len(provider.Content); fieldIndex += 2 {
			key, value := provider.Content[fieldIndex], provider.Content[fieldIndex+1]
			if !isStringScalar(key) || key.Value != "path" {
				continue
			}
			value = resolveAlias(value)
			if !isStringScalar(value) {
				return fmt.Errorf("provider path must be a string")
			}
			if visit != nil {
				visit(value)
			}
		}
	}
	return nil
}

func resolveAlias(node *yaml.Node) *yaml.Node {
	seen := map[*yaml.Node]bool{}
	for node != nil && node.Kind == yaml.AliasNode && node.Alias != nil && !seen[node] {
		seen[node] = true
		node = node.Alias
	}
	return node
}

func relativeProviderPath(value string) bool {
	if value == "" || filepath.IsAbs(value) {
		return false
	}
	return !strings.Contains(value, "://")
}

func stringNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func quotedStringNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value, Style: yaml.DoubleQuotedStyle}
}

func intNode(value int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprintf("%d", value)}
}

func sequenceNode(values ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: values}
}

func mappingNode(values ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: values}
}
