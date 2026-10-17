package mihomo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
)

const MaxCustomDNSSize = 2 << 20
const customDNSMetadataKey = "x-opensurge-custom-dns"

// CustomDNSDocument is a single optional draft. Only YAML is portable; the
// envelope belongs to the control plane and travels in immutable candidates.
type CustomDNSDocument struct {
	SchemaVersion int    `json:"schema_version" yaml:"schema-version"`
	Enabled       bool   `json:"enabled" yaml:"enabled"`
	YAML          string `json:"yaml" yaml:"yaml"`
}

type CustomDNSChange struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
	Source string `json:"source"`
}

type CustomDNSComposition struct {
	ProfileOverlayComposition
	Changes  []CustomDNSChange `json:"changes"`
	Warnings []string          `json:"warnings"`
}

func DefaultCustomDNSDocument() CustomDNSDocument {
	return CustomDNSDocument{SchemaVersion: 1, YAML: "hosts: {}\ndns: {}\n"}
}

func NormalizeCustomDNS(document CustomDNSDocument) (CustomDNSDocument, error) {
	if document.SchemaVersion != 1 {
		return document, fmt.Errorf("unsupported custom DNS schema version %d", document.SchemaVersion)
	}
	root, err := parseCustomDNS([]byte(document.YAML))
	if err != nil {
		return document, err
	}
	document.YAML, err = encodeYAMLNode(root)
	if len(document.YAML) > MaxCustomDNSSize {
		return document, fmt.Errorf("custom DNS exceeds 2 MiB")
	}
	return document, err
}

func CustomDNSDigest(document CustomDNSDocument) string {
	if !document.Enabled {
		return ""
	}
	data, _ := json.Marshal(document)
	return ProfileOverlayDigest(data)
}

func customError(n *yaml.Node, field, message string) error {
	return fmt.Errorf("%s (line %d): %s", field, n.Line, message)
}

// Expand acyclic YAML anchors once, with a bounded budget. This also rejects
// custom tags, null deletion, non-string keys and normalized duplicate keys.
func customNode(node *yaml.Node, stack map[*yaml.Node]bool, budget *int, depth int) (*yaml.Node, error) {
	*budget--
	if *budget < 0 || depth > 64 {
		return nil, fmt.Errorf("custom DNS YAML is too complex")
	}
	if node == nil || stack[node] {
		return nil, fmt.Errorf("custom DNS YAML contains a cyclic alias")
	}
	stack[node] = true
	defer delete(stack, node)
	if node.Kind == yaml.AliasNode {
		return customNode(node.Alias, stack, budget, depth+1)
	}
	if node.Tag != "!!map" && node.Tag != "!!seq" && node.Tag != "!!str" && node.Tag != "!!bool" && node.Tag != "!!int" && node.Tag != "!!float" {
		return nil, customError(node, "yaml", "unsupported tag or null value")
	}
	out := *node
	if out.Tag == "!!bool" {
		out.Value = strings.ToLower(out.Value)
	}
	out.Anchor, out.Alias, out.Content = "", nil, nil
	out.HeadComment, out.LineComment, out.FootComment = "", "", ""
	seen := map[string]bool{}
	for i, child := range node.Content {
		copy, err := customNode(child, stack, budget, depth+1)
		if err != nil {
			return nil, err
		}
		if node.Kind == yaml.MappingNode && i%2 == 0 {
			if !isStringScalar(copy) || copy.Value == "" || copy.Value == "<<" {
				return nil, customError(copy, "yaml", "mapping keys must be non-empty strings; merge keys are unsupported")
			}
			if seen[copy.Value] {
				return nil, customError(copy, copy.Value, "duplicate key")
			}
			seen[copy.Value] = true
		}
		out.Content = append(out.Content, copy)
	}
	return &out, nil
}

func parseCustomDNS(data []byte) (*yaml.Node, error) {
	if len(data) > MaxCustomDNSSize {
		return nil, fmt.Errorf("custom DNS exceeds 2 MiB")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("custom DNS must be UTF-8")
	}
	root, err := decodeSingleYAMLMapping(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	if err != nil {
		return nil, fmt.Errorf("custom DNS: %w", err)
	}
	budget := 200000
	root, err = customNode(root, map[*yaml.Node]bool{}, &budget, 0)
	if err != nil {
		return nil, err
	}
	if len(root.Content) == 0 {
		return nil, fmt.Errorf("custom DNS requires hosts or dns")
	}
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Value != "hosts" && key.Value != "dns" {
			return nil, customError(key, key.Value, "only hosts and dns are supported")
		}
		if value.Kind != yaml.MappingNode {
			return nil, customError(value, key.Value, "must be a mapping")
		}
		if key.Value == "hosts" {
			err = validateCustomHosts(value)
		} else {
			err = validateCustomDNSFields(value)
		}
		if err != nil {
			return nil, err
		}
	}
	return root, nil
}

func customValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil {
		return nil
	}
	if i := mappingValueIndex(n, key); i >= 0 {
		return resolveAlias(n.Content[i])
	}
	return nil
}

// Domain patterns use whole-label wildcards, as supported by mihomo's trie.
// A leading dot matches subdomains; +. includes the apex as well.
func normalizeCustomDomain(value string, pattern bool) (string, error) {
	value = strings.TrimSuffix(strings.ToLower(value), ".")
	if value == "" || len(value) > 253 {
		return "", fmt.Errorf("invalid domain name")
	}
	domain := value
	if pattern {
		domain = strings.TrimPrefix(domain, "+.")
		domain = strings.TrimPrefix(domain, ".")
	}
	for _, label := range strings.Split(domain, ".") {
		if pattern && label == "*" {
			continue
		}
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid domain name %q", value)
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return "", fmt.Errorf("use ASCII/punycode domain names and whole-label wildcards: %q", value)
			}
		}
	}
	return value, nil
}

func normalizeCustomKeys(mapping *yaml.Node, field string) error {
	seen := map[string]bool{}
	for i := 0; i < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		value, err := normalizeCustomDomain(k.Value, true)
		if err != nil {
			return customError(k, field, err.Error())
		}
		if seen[value] {
			return customError(k, field+"."+value, "duplicate normalized domain")
		}
		seen[value] = true
		k.Value = value
	}
	return nil
}

func validateCustomHosts(hosts *yaml.Node) error {
	if err := normalizeCustomKeys(hosts, "hosts"); err != nil {
		return err
	}
	for i := 0; i < len(hosts.Content); i += 2 {
		k, v := hosts.Content[i], hosts.Content[i+1]
		values := []*yaml.Node{v}
		if v.Kind == yaml.SequenceNode {
			if len(v.Content) == 0 {
				return customError(v, "hosts."+k.Value, "IP list cannot be empty")
			}
			values = v.Content
		}
		seen := map[string]bool{}
		unique := []*yaml.Node{}
		for _, item := range values {
			if !isStringScalar(item) {
				return customError(item, "hosts."+k.Value, "expected an IP address or a domain alias")
			}
			ip, err := netip.ParseAddr(item.Value)
			if err == nil && ip.Zone() == "" {
				item.Value = ip.String()
			} else {
				if v.Kind == yaml.SequenceNode {
					return customError(item, "hosts."+k.Value, "arrays may contain only IP addresses")
				}
				name, e := normalizeCustomDomain(item.Value, false)
				if e != nil {
					return customError(item, "hosts."+k.Value, e.Error())
				}
				item.Value = name
			}
			if !seen[item.Value] {
				unique = append(unique, item)
				seen[item.Value] = true
			}
		}
		if v.Kind == yaml.SequenceNode {
			v.Content = unique
		}
	}
	return nil
}

func customStrings(n *yaml.Node, field string, scalar bool, validate func(string) (string, error)) error {
	values := n.Content
	if scalar && isStringScalar(n) {
		values = []*yaml.Node{n}
	} else if n.Kind != yaml.SequenceNode {
		return customError(n, field, "must be a list of strings")
	}
	for _, item := range values {
		if !isStringScalar(item) || strings.TrimSpace(item.Value) != item.Value || item.Value == "" {
			return customError(item, field, "expected a non-empty trimmed string")
		}
		if validate != nil {
			v, err := validate(item.Value)
			if err != nil {
				return customError(item, field, err.Error())
			}
			item.Value = v
		}
	}
	return nil
}

func validateCustomDNSFields(dns *yaml.Node) error {
	for i := 0; i < len(dns.Content); i += 2 {
		key, v := dns.Content[i].Value, dns.Content[i+1]
		field := "dns." + key
		if !profileOverlayDNSFields[key] {
			return customError(dns.Content[i], field, "unsupported or gateway-owned field")
		}
		var err error
		switch key {
		case "use-hosts", "use-system-hosts", "respect-rules", "prefer-h3", "direct-nameserver-follow-policy":
			if v.Tag != "!!bool" {
				err = fmt.Errorf("must be true or false")
			}
		case "cache-algorithm":
			if !isStringScalar(v) || (v.Value != "lru" && v.Value != "arc") {
				err = fmt.Errorf("must be lru or arc")
			}
		case "fake-ip-filter-mode":
			if !isStringScalar(v) || (v.Value != "blacklist" && v.Value != "whitelist" && v.Value != "rule") {
				err = fmt.Errorf("must be blacklist, whitelist or rule")
			}
		case "nameserver-policy", "proxy-server-nameserver-policy":
			if v.Kind != yaml.MappingNode {
				err = fmt.Errorf("must be a mapping")
				break
			}
			if err = normalizeCustomKeys(v, field); err != nil {
				return err
			}
			for j := 0; j < len(v.Content); j += 2 {
				if err = customStrings(v.Content[j+1], field+"."+v.Content[j].Value, true, validateCustomResolver); err != nil {
					return err
				}
			}
		case "fake-ip-filter":
			err = customStrings(v, field, false, nil) // Mode-dependent syntax is checked after inheritance.
			if err == nil {
				for _, item := range v.Content {
					if _, e := normalizeCustomDomain(item.Value, true); e != nil {
						if e = validateCustomFilterRule(item.Value); e != nil {
							return customError(item, field, e.Error())
						}
					}
				}
			}
		case "fallback-filter":
			if v.Kind != yaml.MappingNode {
				err = fmt.Errorf("must be a mapping")
				break
			}
			for j := 0; j < len(v.Content); j += 2 {
				f, n := v.Content[j].Value, v.Content[j+1]
				switch f {
				case "geoip":
					if n.Tag != "!!bool" {
						err = fmt.Errorf("geoip must be boolean")
					}
				case "geoip-code":
					if !isStringScalar(n) || len(n.Value) != 2 {
						err = fmt.Errorf("geoip-code must have two letters")
					}
				case "ipcidr":
					err = customStrings(n, field+".ipcidr", false, func(s string) (string, error) { _, e := netip.ParsePrefix(s); return s, e })
				case "domain":
					err = customStrings(n, field+".domain", false, func(s string) (string, error) { return normalizeCustomDomain(s, true) })
				default:
					err = fmt.Errorf("unsupported fallback-filter field %q; geosite/provider dependencies are not supported", f)
				}
				if err != nil {
					return customError(n, field+"."+f, err.Error())
				}
			}
		default:
			err = customStrings(v, field, false, validateCustomResolver)
		}
		if err != nil {
			return customError(v, field, err.Error())
		}
	}
	return nil
}

func validateCustomResolver(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n\t ") || strings.HasPrefix(value, "/") || strings.Contains(value, "file:") {
		return "", fmt.Errorf("expected a DNS resolver, not a file or directive")
	}
	// Protocol/address and proxy parameter semantics are validated by the same
	// reference binder and engine validation used for imported DNS fields.
	return value, nil
}

func validateCustomFilterRule(value string) error {
	p := strings.Split(value, ",")
	if len(p) < 2 || (p[len(p)-1] != "fake-ip" && p[len(p)-1] != "real-ip") {
		return fmt.Errorf("expected a domain filter or DOMAIN/DOMAIN-SUFFIX/DOMAIN-KEYWORD/MATCH rule with real-ip or fake-ip")
	}
	switch p[0] {
	case "MATCH":
		if len(p) == 2 {
			return nil
		}
	case "DOMAIN", "DOMAIN-SUFFIX":
		if len(p) == 3 {
			_, err := normalizeCustomDomain(p[1], false)
			return err
		}
	case "DOMAIN-KEYWORD":
		if len(p) == 3 && p[1] != "" && !strings.ContainsAny(p[1], " \t\r\n:/") {
			return nil
		}
	}
	return fmt.Errorf("unsupported fake-ip-filter rule %q", value)
}

// ImportCustomDNS replaces only sections present in an import. It never saves
// anything; callers display the replacement and explicitly accept it first.
func ImportCustomDNS(current CustomDNSDocument, format string, data []byte) (CustomDNSDocument, []string, error) {
	current, err := NormalizeCustomDNS(current)
	if err != nil {
		return current, nil, err
	}
	var incoming *yaml.Node
	switch format {
	case "yaml":
		incoming, err = parseCustomDNS(data)
	case "hosts":
		incoming, err = importCustomHosts(data)
	default:
		err = fmt.Errorf("import format must be hosts or yaml")
	}
	if err != nil {
		return current, nil, err
	}
	root, _ := parseCustomDNS([]byte(current.YAML))
	sections := []string{}
	for i := 0; i < len(incoming.Content); i += 2 {
		key := incoming.Content[i].Value
		setCustomNode(root, key, incoming.Content[i+1])
		sections = append(sections, key)
	}
	current.YAML, err = encodeYAMLNode(root)
	if err != nil {
		return current, nil, err
	}
	current, err = NormalizeCustomDNS(current)
	return current, sections, err
}

func importCustomHosts(data []byte) (*yaml.Node, error) {
	if len(data) > MaxCustomDNSSize {
		return nil, fmt.Errorf("hosts file exceeds 2 MiB")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("hosts file must be UTF-8")
	}
	hosts := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	indices := map[string]int{}
	for i, line := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		fields := strings.Fields(strings.SplitN(line, "#", 2)[0])
		if len(fields) == 0 {
			continue
		}
		if len(fields) < 2 {
			return nil, fmt.Errorf("hosts line %d: expected IP and one or more names", i+1)
		}
		ip, err := netip.ParseAddr(fields[0])
		if err != nil || ip.Zone() != "" {
			return nil, fmt.Errorf("hosts line %d: expected literal IPv4 or IPv6", i+1)
		}
		for _, name := range fields[1:] {
			name, err = normalizeCustomDomain(name, false)
			if err != nil {
				return nil, fmt.Errorf("hosts line %d: %w", i+1, err)
			}
			index, exists := indices[name]
			if !exists {
				hosts.Content = append(hosts.Content, stringNode(name), stringNode(ip.String()))
				indices[name] = len(hosts.Content) - 1
				continue
			}
			value := hosts.Content[index]
			if value.Kind != yaml.SequenceNode {
				value = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{value}}
				hosts.Content[index] = value
			}
			found := false
			for _, n := range value.Content {
				if n.Value == ip.String() {
					found = true
				}
			}
			if !found {
				value.Content = append(value.Content, stringNode(ip.String()))
			}
		}
	}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{stringNode("hosts"), hosts}}, nil
}

func setCustomNode(root *yaml.Node, key string, node *yaml.Node) {
	if i := mappingValueIndex(root, key); i >= 0 {
		root.Content[i] = node
	} else {
		root.Content = append(root.Content, stringNode(key), node)
	}
}

func customText(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	s, _ := encodeYAMLNode(n)
	return s
}

func ComposeCustomDNS(source []byte, document CustomDNSDocument) (CustomDNSComposition, error) {
	result := CustomDNSComposition{Changes: []CustomDNSChange{}, Warnings: []string{}}
	document, err := NormalizeCustomDNS(document)
	if err != nil {
		return result, err
	}
	// Never accept control-plane ownership metadata from a subscription.
	root, err := decodeSingleYAMLMapping(source)
	if err != nil {
		return result, err
	}
	if i := mappingValueIndex(root, customDNSMetadataKey); i >= 0 {
		root.Content = append(root.Content[:i-1], root.Content[i+1:]...)
		s, e := encodeYAMLNode(root)
		if e != nil {
			return result, e
		}
		source = []byte(s)
	}
	if !document.Enabled {
		inspection, e := InspectImportedProfile(source)
		result.ProfileOverlayComposition = ProfileOverlayComposition{ProfileYAML: string(source), Inspection: inspection, Digest: ProfileOverlayDigest(source)}
		return result, e
	}
	base, err := parseImportedProfile(source)
	if err != nil {
		return result, err
	}
	patch, _ := parseCustomDNS([]byte(document.YAML))
	dns, err := decodeSingleYAMLMapping([]byte(base.dnsResolverFields))
	if err != nil {
		return result, err
	}
	record := func(field string, before, after *yaml.Node) {
		result.Changes = append(result.Changes, CustomDNSChange{Field: field, Before: customText(before), After: customText(after), Source: "custom-dns"})
	}
	hosts := customValue(patch, "hosts")
	if hosts != nil && len(hosts.Content) > 0 {
		target := ensureImportedSection(&base, "hosts", yaml.MappingNode, "!!map")
		indices := customDomainIndices(target)
		for i := 0; i < len(hosts.Content); i += 2 {
			key, v := hosts.Content[i].Value, hosts.Content[i+1]
			var before *yaml.Node
			if index, ok := indices[key]; ok {
				before = target.Content[index]
				target.Content[index-1].Value = key
				target.Content[index] = v
			} else {
				target.Content = append(target.Content, stringNode(key), v)
				indices[key] = len(target.Content) - 1
			}
			record("hosts."+key, before, v)
		}
	}
	patchDNS := customValue(patch, "dns")
	if patchDNS != nil {
		mode := "blacklist"
		if n := customValue(dns, "fake-ip-filter-mode"); n != nil {
			mode = n.Value
		}
		next := mode
		if n := customValue(patchDNS, "fake-ip-filter-mode"); n != nil {
			next = n.Value
		}
		filter := customValue(patchDNS, "fake-ip-filter")
		if mode != next && filter == nil {
			return result, fmt.Errorf("dns.fake-ip-filter: changing mode requires an explicit complete filter list")
		}
		if filter != nil {
			for i, n := range filter.Content {
				if next == "rule" {
					if e := validateCustomFilterRule(n.Value); e != nil {
						return result, customError(n, "dns.fake-ip-filter", e.Error())
					}
					if strings.HasPrefix(n.Value, "MATCH,") && i != len(filter.Content)-1 {
						return result, customError(n, "dns.fake-ip-filter", "MATCH must be the last custom rule")
					}
				} else {
					v, e := normalizeCustomDomain(n.Value, true)
					if e != nil {
						return result, customError(n, "dns.fake-ip-filter", e.Error())
					}
					n.Value = v
				}
			}
			before := customValue(dns, "fake-ip-filter")
			merged := sequenceNode()
			if mode == next {
				list := []*yaml.Node{}
				if before != nil {
					if before.Kind != yaml.SequenceNode {
						return result, fmt.Errorf("inherited dns.fake-ip-filter must be a list")
					}
					list = append(list, before.Content...)
				}
				if next == "rule" {
					list = append(append([]*yaml.Node{}, filter.Content...), list...)
				} else {
					list = append(list, filter.Content...)
				}
				seen := map[string]bool{}
				for _, n := range list {
					if !seen[n.Value] {
						merged.Content = append(merged.Content, n)
						seen[n.Value] = true
					}
				}
				if next == "rule" {
					for _, n := range filter.Content {
						if strings.HasPrefix(n.Value, "MATCH,") && before != nil && len(before.Content) > 0 {
							result.Warnings = append(result.Warnings, "custom MATCH shadows inherited fake-ip-filter rules")
						}
					}
				}
			} else {
				merged.Content = filter.Content
				result.Warnings = append(result.Warnings, "fake-ip-filter mode changed: inherited mode and filter list are replaced")
			}
			record("dns.fake-ip-filter", before, merged)
			setCustomNode(dns, "fake-ip-filter", merged)
		}
		for i := 0; i < len(patchDNS.Content); i += 2 {
			key, v := patchDNS.Content[i].Value, patchDNS.Content[i+1]
			if key == "fake-ip-filter" {
				continue
			}
			before := customText(customValue(dns, key))
			old := customValue(dns, key)
			if old != nil && old.Kind == yaml.MappingNode && v.Kind == yaml.MappingNode {
				if key == "nameserver-policy" || key == "proxy-server-nameserver-policy" {
					indices := customDomainIndices(old)
					for j := 0; j < len(v.Content); j += 2 {
						k, value := v.Content[j].Value, v.Content[j+1]
						var previous *yaml.Node
						if index, ok := indices[k]; ok {
							previous = old.Content[index]
							old.Content[index-1].Value, old.Content[index] = k, value
						} else {
							old.Content = append(old.Content, stringNode(k), value)
							indices[k] = len(old.Content) - 1
						}
						record("dns."+key+"."+k, previous, value)
					}
				} else {
					deepMergeYAMLMapping(old, v)
				}
			} else {
				setCustomNode(dns, key, v)
			}
			result.Changes = append(result.Changes, CustomDNSChange{Field: "dns." + key, Before: before, After: customText(customValue(dns, key)), Source: "custom-dns"})
		}
	}
	if hosts != nil && len(hosts.Content) > 0 {
		if n := customValue(patchDNS, "use-hosts"); n != nil && n.Value == "false" {
			return result, fmt.Errorf("dns.use-hosts: false conflicts with custom hosts")
		}
		before := customValue(dns, "use-hosts")
		after := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
		record("dns.use-hosts", before, after)
		setCustomNode(dns, "use-hosts", after)
		result.Warnings = append(result.Warnings, "hosts is evaluated before upstream policy; more specific inherited domains can override custom wildcards; hosts also affects node/provider resolution")
	}
	if n := customValue(dns, "direct-nameserver"); n != nil && len(n.Content) > 0 {
		follow := "false"
		if f := customValue(dns, "direct-nameserver-follow-policy"); f != nil {
			follow = f.Value
		}
		result.Warnings = append(result.Warnings, "direct-nameserver may resolve again when connecting; direct-nameserver-follow-policy="+follow)
	}
	result.Warnings = append(result.Warnings, customDNSNodeWarnings(&base, hosts)...)
	for _, mapping := range []*yaml.Node{hosts, customValue(patchDNS, "nameserver-policy"), customValue(patchDNS, "proxy-server-nameserver-policy")} {
		if mapping == nil {
			continue
		}
		for i := 0; i < len(mapping.Content); i += 2 {
			name := mapping.Content[i].Value
			if name == "local" || strings.HasSuffix(name, ".local") {
				result.Warnings = append(result.Warnings, ".local names may use macOS mDNS and bypass unicast DNS; this feature does not manage mDNS")
				break
			}
		}
	}
	if err = validateCustomAliasChains(base.sections["hosts"], hosts, config.Config{}); err != nil {
		return result, err
	}
	if err = refreshDNSOrigins(base.dnsSource, dns, "custom-dns", CustomDNSDigest(document)); err != nil {
		return result, err
	}
	payload, err := renderComposedProfileSource(&base, dns)
	if err != nil {
		return result, err
	}
	root, err = decodeSingleYAMLMapping(payload)
	if err != nil {
		return result, err
	}
	setPreparedField(root, customDNSMetadataKey, document)
	rendered, err := encodeYAMLNode(root)
	if err != nil {
		return result, err
	}
	inspection, err := InspectImportedProfile([]byte(rendered))
	result.ProfileOverlayComposition = ProfileOverlayComposition{ProfileYAML: rendered, Inspection: inspection, Digest: ProfileOverlayDigest([]byte(rendered))}
	return result, err
}

func CustomDNSFromProfile(data []byte) (CustomDNSDocument, error) {
	root, err := decodeSingleYAMLMapping(data)
	if err != nil {
		return CustomDNSDocument{}, err
	}
	n := customValue(root, customDNSMetadataKey)
	if n == nil {
		return DefaultCustomDNSDocument(), nil
	}
	var doc CustomDNSDocument
	if err = n.Decode(&doc); err != nil {
		return doc, err
	}
	if !doc.Enabled {
		return doc, fmt.Errorf("invalid disabled custom DNS candidate metadata")
	}
	return NormalizeCustomDNS(doc)
}

// ValidateCustomDNSCandidate is rerun against current gateway settings, even
// for an unchanged materialized profile. DHCP namespace changes cannot bypass
// the dual-entry contract by reusing an old composition digest.
func ValidateCustomDNSCandidate(cfg config.Config, data []byte) error {
	doc, err := CustomDNSFromProfile(data)
	if err != nil {
		return err
	}
	if CustomDNSDigest(doc) != cfg.Mihomo.ProfileCustomDNSDigest {
		return fmt.Errorf("custom DNS candidate metadata does not match the selected configuration")
	}
	if !doc.Enabled {
		return nil
	}
	if err = ValidateCustomDNSEntries(cfg); err != nil {
		return err
	}
	patch, _ := parseCustomDNS([]byte(doc.YAML))
	if cfg.DHCP.Enabled {
		for _, m := range []*yaml.Node{customValue(patch, "hosts"), customValue(customValue(patch, "dns"), "nameserver-policy"), customValue(customValue(patch, "dns"), "proxy-server-nameserver-policy")} {
			if m == nil {
				continue
			}
			for i := 0; i < len(m.Content); i += 2 {
				if customOverlapsDHCP(m.Content[i].Value, cfg.DHCP.Domain) {
					return fmt.Errorf("custom DNS domain %q overlaps DHCP single-label or %q lease names", m.Content[i].Value, cfg.DHCP.Domain)
				}
			}
		}
	}
	root, _ := decodeSingleYAMLMapping(data)
	return validateCustomAliasChains(customValue(root, "hosts"), customValue(patch, "hosts"), cfg)
}

func ValidateCustomDNSEntries(cfg config.Config) error {
	upstream := strings.TrimSpace(cfg.DNS.Upstream)
	if upstream != "" && upstream != config.MihomoDNSUpstream {
		return fmt.Errorf("custom DNS requires the managed DNS upstream in Network settings")
	}
	if !cfg.ManageSystemDNS() {
		return fmt.Errorf("custom DNS requires TUN, automatic routing and Mac system DNS coordination in Network settings")
	}
	return nil
}

// Conservative intersection check with the entire future lease namespace.
// Compare literal suffix labels; wildcards are never evidence of disjointness.
func customOverlapsDHCP(pattern, domain string) bool {
	pattern = strings.ToLower(pattern)
	p := strings.TrimPrefix(strings.TrimPrefix(pattern, "+."), ".")
	labels := strings.Split(p, ".")
	if len(labels) == 1 && !strings.HasPrefix(pattern, ".") {
		return true
	}
	domain = strings.Trim(strings.ToLower(domain), ". ")
	if domain == "" {
		return false
	}
	if !strings.Contains(pattern, "*") && !strings.HasPrefix(pattern, ".") && !strings.HasPrefix(pattern, "+.") {
		return p == domain || strings.HasSuffix(p, "."+domain)
	}
	suffix := strings.Split(domain, ".")
	if len(labels) < len(suffix) && !strings.HasPrefix(pattern, ".") && !strings.HasPrefix(pattern, "+.") {
		return false
	}
	for i, j := len(labels)-1, len(suffix)-1; i >= 0 && j >= 0; i, j = i-1, j-1 {
		if labels[i] != "*" && labels[i] != suffix[j] {
			return false
		}
	}
	return true
}

// Resolve alias reachability conservatively through every potentially matching
// wildcard, including inherited hosts. Unrelated legacy aliases are untouched.
func validateCustomAliasChains(all, custom *yaml.Node, cfg config.Config) error {
	if all == nil || custom == nil {
		return nil
	}
	exact := map[string]*yaml.Node{}
	type wildcard struct {
		key   string
		value *yaml.Node
	}
	wildcards := []wildcard{}
	for i := 0; i < len(all.Content); i += 2 {
		key := strings.ToLower(strings.TrimSuffix(all.Content[i].Value, "."))
		value := resolveAlias(all.Content[i+1])
		if strings.ContainsAny(key, "*+") || strings.HasPrefix(key, ".") {
			wildcards = append(wildcards, wildcard{key, value})
		} else {
			exact[key] = value
		}
	}
	done := map[string]bool{}
	budget := 200000
	var visit func(string, map[string]bool, int) error
	visit = func(name string, stack map[string]bool, depth int) error {
		if depth > 64 || stack[name] {
			return fmt.Errorf("custom hosts alias cycle or excessive depth at %q", name)
		}
		if cfg.DHCP.Enabled && customOverlapsDHCP(name, cfg.DHCP.Domain) {
			return fmt.Errorf("custom hosts alias %q overlaps DHCP lease names", name)
		}
		if done[name] {
			return nil
		}
		stack[name] = true
		defer delete(stack, name)
		matches := []*yaml.Node{}
		bestRank := ""
		if value, ok := exact[name]; ok {
			matches = append(matches, value)
			bestRank = strings.Repeat("2", len(strings.Split(name, ".")))
		}
		for _, entry := range wildcards {
			budget--
			if budget < 0 {
				return fmt.Errorf("custom hosts alias graph is too complex to verify")
			}
			rank := customDomainRank(entry.key, name)
			if rank != "" && rank >= bestRank {
				if rank > bestRank {
					matches = nil
					bestRank = rank
				}
				// +. inserts an apex too. If it collides with an exact key,
				// validate both possible values rather than assume map order.
				matches = append(matches, entry.value)
			}
		}
		for _, v := range matches {
			if !isStringScalar(v) {
				continue
			}
			if _, err := netip.ParseAddr(v.Value); err == nil {
				continue
			}
			target, err := normalizeCustomDomain(v.Value, false)
			if err != nil {
				return fmt.Errorf("custom hosts alias reaches unsupported inherited target %q", v.Value)
			}
			if err = visit(target, stack, depth+1); err != nil {
				return err
			}
		}
		done[name] = true
		return nil
	}
	for i := 0; i < len(custom.Content); i += 2 {
		v := custom.Content[i+1]
		if !isStringScalar(v) {
			continue
		}
		if _, err := netip.ParseAddr(v.Value); err == nil {
			continue
		}
		if err := visit(v.Value, map[string]bool{}, 0); err != nil {
			return err
		}
	}
	return nil
}

func customDomainMatches(pattern, name string) bool { return customDomainRank(pattern, name) != "" }

// Mihomo's domain trie prefers exact labels over * and then suffix matches,
// proceeding from the rightmost label. Equal patterns remain conservative.
func customDomainRank(pattern, name string) string {
	apex := strings.HasPrefix(pattern, "+.")
	suffix := apex || strings.HasPrefix(pattern, ".")
	pattern = strings.TrimPrefix(strings.TrimPrefix(pattern, "+."), ".")
	p, n := strings.Split(pattern, "."), strings.Split(name, ".")
	if len(n) < len(p) || (!suffix && len(n) != len(p)) || (suffix && !apex && len(n) == len(p)) {
		return ""
	}
	rank := bytes.Repeat([]byte{'0'}, len(n))
	for i := 0; i < len(p); i++ {
		label := p[len(p)-1-i]
		if label == "*" {
			rank[i] = '1'
		} else if label == n[len(n)-1-i] {
			rank[i] = '2'
		} else {
			return ""
		}
	}
	return string(rank)
}

func CustomDNSCounts(doc CustomDNSDocument) (int, int) {
	root, err := parseCustomDNS([]byte(doc.YAML))
	if err != nil {
		return 0, 0
	}
	hosts, policies := 0, 0
	if n := customValue(root, "hosts"); n != nil {
		hosts = len(n.Content) / 2
	}
	dns := customValue(root, "dns")
	for _, key := range []string{"nameserver-policy", "proxy-server-nameserver-policy"} {
		if n := customValue(dns, key); n != nil {
			policies += len(n.Content) / 2
		}
	}
	return hosts, policies
}

// Preview/export never exposes proxy credentials or control-plane metadata.
func CustomDNSProfileSection(data []byte) string {
	root, err := decodeSingleYAMLMapping(data)
	if err != nil {
		return ""
	}
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, key := range []string{"hosts", "dns"} {
		if n := customValue(root, key); n != nil {
			setCustomNode(out, key, n)
		}
	}
	return customText(out)
}

func CustomDNSSectionChanges(before, after []byte, source string) []CustomDNSChange {
	changes := []CustomDNSChange{}
	a, _ := decodeSingleYAMLMapping(before)
	b, err := decodeSingleYAMLMapping(after)
	if err != nil {
		return changes
	}
	for _, section := range []string{"hosts", "dns"} {
		old, next := customValue(a, section), customValue(b, section)
		if next == nil {
			continue
		}
		for i := 0; i < len(next.Content); i += 2 {
			key := next.Content[i].Value
			x, y := customText(customValue(old, key)), customText(next.Content[i+1])
			if x != y {
				changes = append(changes, CustomDNSChange{Field: section + "." + key, Before: x, After: y, Source: source})
			}
		}
	}
	return changes
}

func customDomainIndices(mapping *yaml.Node) map[string]int {
	indices := map[string]int{}
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i].Value
		if n, err := normalizeCustomDomain(key, true); err == nil {
			indices[n] = i + 1
		}
	}
	return indices
}

func customDNSNodeWarnings(base *importedProfile, hosts *yaml.Node) []string {
	warnings := []string{}
	if hosts == nil {
		return warnings
	}
	check := func(kind, name, server string) {
		if server == "" {
			return
		}
		for i := 0; i < len(hosts.Content); i += 2 {
			if customDomainMatches(hosts.Content[i].Value, strings.ToLower(server)) {
				warnings = append(warnings, fmt.Sprintf("custom hosts %q matches %s %q (%s)", hosts.Content[i].Value, kind, name, server))
				break
			}
		}
	}
	if proxies := resolveAlias(base.sections["proxies"]); proxies != nil {
		for _, proxy := range proxies.Content {
			proxy = resolveAlias(proxy)
			name, server := customValue(proxy, "name"), customValue(proxy, "server")
			if name != nil && server != nil {
				check("node", name.Value, server.Value)
			}
		}
	}
	if providers := resolveAlias(base.sections["proxy-providers"]); providers != nil {
		for i := 0; i < len(providers.Content); i += 2 {
			provider := resolveAlias(providers.Content[i+1])
			if u := customValue(provider, "url"); u != nil {
				if parsed, err := url.Parse(u.Value); err == nil {
					check("provider", providers.Content[i].Value, parsed.Hostname())
				}
			}
		}
	}
	return warnings
}

func validateCustomDNSForwardingLoops(cfg config.Config, fields string) error {
	if cfg.Mihomo.ProfileCustomDNSDigest == "" {
		return nil
	}
	root, err := decodeSingleYAMLMapping([]byte(fields))
	if err != nil {
		return err
	}
	targets, err := dnsTargets(root)
	if err != nil {
		return err
	}
	for _, target := range targets {
		addr := parseDNSAddress(target.node.Value)
		if ip, e := netip.ParseAddr(target.node.Value); e == nil && ip.Zone() == "" {
			addr = dnsAddress{host: ip.String(), port: "53", scheme: "udp", ip: ip, simple: true}
		}
		if addr.scheme != "udp" && addr.scheme != "tcp" {
			continue
		}
		host := strings.ToLower(addr.host)
		ownLAN := host == cfg.DNS.Listen || host == cfg.Gateway.LANIP || host == config.DownstreamIPv6Gateway
		// The public system-DNS interception address is not a local listener:
		// explicit engine resolvers use the managed outbound interface.
		if ownLAN && addr.port == strconv.Itoa(cfg.DNS.Port) {
			return fmt.Errorf("%s: resolver %q points back to the managed gateway DNS and would create a forwarding loop", target.field, target.node.Value)
		}
	}
	return nil
}
