package mihomo

import (
	"bytes"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const dnsSourceMetadataKey = "x-opensurge-dns-source"
const dnsBindingsKey = "x-opensurge-dns-bindings"
const dnsServicesKey = "x-opensurge-dns-services"
const dnsProbeKey = "x-opensurge-dns-probe"
const gatewayDNSEndpoint = "127.0.0.1:1053"

// DNSServiceDeclaration belongs to the selected source and an exact field/value
// pair. It is independent of any optional editor's enabled flag. Changing the
// final target invalidates the match; declarations never rewrite other values.
type DNSServiceDeclaration struct {
	Field   string `json:"field" yaml:"field"`
	Target  string `json:"target" yaml:"target"`
	Service string `json:"service" yaml:"service"`
}

type dnsReferenceOrigin struct {
	Target    string              `json:"target" yaml:"target"`
	Source    string              `json:"source" yaml:"source"`
	Digest    string              `json:"digest" yaml:"digest"`
	Overrides *dnsReferenceOrigin `json:"overrides,omitempty" yaml:"overrides,omitempty"`
}

// Materialized profiles keep this metadata outside dns: the original listener
// remains evidence, never permission to restore a subscription's ingress.
type dnsSourceMetadata struct {
	Version      int                           `yaml:"version"`
	SourceDigest string                        `yaml:"source-digest"`
	Enabled      bool                          `yaml:"enabled"`
	Listen       string                        `yaml:"listen"`
	Origins      map[string]dnsReferenceOrigin `yaml:"origins"`
	Services     []DNSServiceDeclaration       `yaml:"services,omitempty"`
}

// DNSReference is a configuration inference, not evidence of a live query.
// The runtime copy also identifies exactly which fields preparation may rebind.
type DNSReference struct {
	Field        string              `json:"field" yaml:"field"`
	Target       string              `json:"target" yaml:"target"`
	Effective    string              `json:"effective" yaml:"effective"`
	Source       string              `json:"source" yaml:"source"`
	SourceDigest string              `json:"source_digest" yaml:"source-digest"`
	Purpose      string              `json:"purpose" yaml:"purpose"`
	Service      string              `json:"service" yaml:"service"`
	Overrides    *dnsReferenceOrigin `json:"overrides,omitempty" yaml:"overrides,omitempty"`
}

type dnsTarget struct {
	field, purpose string
	node           *yaml.Node
}

func dnsValue(root *yaml.Node, name string) *yaml.Node {
	if root != nil && root.Kind == yaml.MappingNode {
		if i := mappingValueIndex(root, name); i >= 0 {
			return resolveAlias(root.Content[i])
		}
	}
	return nil
}

func decodeDNSMetadata(node *yaml.Node, out any) error {
	data, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	return decoder.Decode(out)
}

func captureDNSSource(root *yaml.Node, source []byte) (*dnsSourceMetadata, bool, error) {
	meta := &dnsSourceMetadata{Version: 1, SourceDigest: ProfileOverlayDigest(source), Origins: map[string]dnsReferenceOrigin{}}
	stored := dnsValue(root, dnsSourceMetadataKey)
	if stored != nil {
		if err := decodeDNSMetadata(stored, meta); err != nil {
			return nil, false, fmt.Errorf("DNS source metadata: %w", err)
		}
		if meta.Version != 1 || !validDNSDigest(meta.SourceDigest) {
			return nil, false, fmt.Errorf("invalid DNS source metadata version or digest")
		}
		if meta.Origins == nil {
			meta.Origins = map[string]dnsReferenceOrigin{}
		}
	} else if dns := dnsValue(root, "dns"); dns != nil {
		if enabled := dnsValue(dns, "enable"); enabled != nil {
			if err := enabled.Decode(&meta.Enabled); err != nil {
				return nil, false, fmt.Errorf("dns.enable: %w", err)
			}
		}
		if listen := dnsValue(dns, "listen"); listen != nil {
			var ok bool
			meta.Listen, ok = scalarStringValue(listen)
			if !ok {
				return nil, false, fmt.Errorf("dns.listen must be a string")
			}
		}
		if err := refreshDNSOrigins(meta, dns, "subscription", meta.SourceDigest); err != nil {
			return nil, false, err
		}
	}
	if services := dnsValue(root, dnsServicesKey); services != nil {
		if stored != nil {
			return nil, false, fmt.Errorf("DNS declarations must be stored in either the original source or its metadata, not both")
		}
		if err := decodeDNSMetadata(services, &meta.Services); err != nil {
			return nil, false, fmt.Errorf("%s: %w", dnsServicesKey, err)
		}
	}
	seen := map[string]bool{}
	for _, declaration := range meta.Services {
		key := declaration.Field + "\x00" + declaration.Target
		if !strings.HasPrefix(declaration.Field, "dns.") || declaration.Target == "" || (declaration.Service != "self" && declaration.Service != "independent") || seen[key] {
			return nil, false, fmt.Errorf("invalid or duplicate DNS service declaration for %s", declaration.Field)
		}
		seen[key] = true
	}
	return meta, stored != nil, nil
}

func validDNSDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func refreshDNSOrigins(meta *dnsSourceMetadata, dns *yaml.Node, source, digest string) error {
	targets, err := dnsTargets(dns)
	if err != nil {
		return err
	}
	origins := make(map[string]dnsReferenceOrigin, len(targets))
	for _, target := range targets {
		origin, ok := meta.Origins[target.field]
		if !ok || origin.Target != target.node.Value {
			previous := origin
			previous.Overrides = nil
			origin = dnsReferenceOrigin{Target: target.node.Value, Source: source, Digest: digest}
			if ok {
				origin.Overrides = &previous
			}
		}
		origins[target.field] = origin
	}
	meta.Origins = origins
	return nil
}

// Resolve aliases into independent nodes before rewriting. A shared YAML anchor
// must not make rebinding one field mutate a different resolver or leave a
// dangling alias after filtering. The hosts section remains untouched.
func copyDNSNode(node *yaml.Node) (*yaml.Node, error) {
	stack := map[*yaml.Node]bool{}
	remaining := 100000
	var clone func(*yaml.Node) (*yaml.Node, error)
	clone = func(current *yaml.Node) (*yaml.Node, error) {
		remaining--
		if remaining < 0 || len(stack) > 100 {
			return nil, fmt.Errorf("DNS YAML alias expansion exceeds the supported limit")
		}
		if current == nil {
			return nil, nil
		}
		if stack[current] {
			return nil, fmt.Errorf("recursive DNS YAML alias is not supported")
		}
		stack[current] = true
		defer delete(stack, current)
		if current.Kind == yaml.AliasNode {
			return clone(current.Alias)
		}
		copy := *current
		copy.Anchor, copy.Alias = "", nil
		copy.Content = make([]*yaml.Node, len(current.Content))
		for i, child := range current.Content {
			var err error
			copy.Content[i], err = clone(child)
			if err != nil {
				return nil, err
			}
		}
		return &copy, nil
	}
	return clone(node)
}

func dnsTargets(dns *yaml.Node) ([]dnsTarget, error) {
	var targets []dnsTarget
	if dns == nil {
		return targets, nil
	}
	if dns.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("dns must be a mapping")
	}
	var collect func(*yaml.Node, string, string) error
	collect = func(node *yaml.Node, path, purpose string) error {
		node = resolveAlias(node)
		if node == nil {
			return fmt.Errorf("%s must contain DNS resolver strings", path)
		}
		if isStringScalar(node) {
			targets = append(targets, dnsTarget{path, purpose, node})
			return nil
		}
		if node.Kind != yaml.SequenceNode {
			return fmt.Errorf("%s must be a resolver string or sequence", path)
		}
		for i, child := range node.Content {
			child = resolveAlias(child)
			if !isStringScalar(child) {
				return fmt.Errorf("%s[%d] must be a DNS resolver string", path, i)
			}
			targets = append(targets, dnsTarget{fmt.Sprintf("%s[%d]", path, i), purpose, child})
		}
		return nil
	}
	for _, field := range []string{"nameserver", "default-nameserver", "fallback", "direct-nameserver", "proxy-server-nameserver", "nameserver-policy", "proxy-server-nameserver-policy"} {
		node := dnsValue(dns, field)
		if node == nil {
			continue
		}
		purpose := field
		if strings.HasSuffix(field, "-policy") {
			if node.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("dns.%s must be a mapping", field)
			}
			for i := 0; i < len(node.Content); i += 2 {
				key, ok := scalarStringValue(node.Content[i])
				if !ok {
					return nil, fmt.Errorf("dns.%s policy key must be a string", field)
				}
				if err := collect(node.Content[i+1], "dns."+field+"["+strconv.Quote(key)+"]", purpose); err != nil {
					return nil, err
				}
			}
		} else if err := collect(node, "dns."+field, purpose); err != nil {
			return nil, err
		}
	}
	return targets, nil
}

type dnsAddress struct {
	host, port, scheme string
	ip                 netip.Addr
	simple             bool
}

func parseDNSAddress(value string) dnsAddress {
	text := value
	if !strings.Contains(text, "://") {
		text = "udp://" + text
	}
	u, err := url.Parse(text)
	if err != nil {
		return dnsAddress{}
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "udp", "tcp":
			port = "53"
		case "tls", "quic":
			port = "853"
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	ip, _ := netip.ParseAddr(u.Hostname())
	n, err := strconv.Atoi(port)
	simple := err == nil && n > 0 && n <= 65535 && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == "" && (u.Scheme == "udp" || u.Scheme == "tcp") && ip.IsValid() && ip.Zone() == ""
	return dnsAddress{u.Hostname(), port, u.Scheme, ip, simple}
}

func bindDNSReferences(fields string, meta *dnsSourceMetadata) (string, []DNSReference, error) {
	dns, err := decodeSingleYAMLMapping([]byte(fields))
	if err != nil {
		return "", nil, err
	}
	dns, err = copyDNSNode(dns)
	if err != nil {
		return "", nil, err
	}
	targets, err := dnsTargets(dns)
	if err != nil {
		return "", nil, err
	}
	var references []DNSReference
	listenHost, listenPort, listenErr := net.SplitHostPort(meta.Listen)
	listenIP, _ := netip.ParseAddr(listenHost)
	for _, target := range targets {
		value := target.node.Value
		address := parseDNSAddress(value)
		origin, ok := meta.Origins[target.field]
		if !ok || origin.Target != value {
			origin = dnsReferenceOrigin{Source: "managed"}
		}
		ref := DNSReference{Field: target.field, Target: value, Effective: value, Source: origin.Source, SourceDigest: origin.Digest, Purpose: target.purpose, Service: "independent"}
		ref.Overrides = origin.Overrides
		declared := ""
		for _, declaration := range meta.Services {
			if declaration.Field == target.field && declaration.Target == value {
				declared = declaration.Service
			}
		}
		local := address.ip.IsLoopback() || address.ip.IsUnspecified() || strings.EqualFold(address.host, "localhost")
		if local && meta.Enabled && meta.Listen != "" && listenErr != nil && declared != "independent" {
			return "", nil, fmt.Errorf("%s has an unsupported original DNS listener %q; declare an independent service or restore a literal listener address", target.field, meta.Listen)
		}
		covered := listenIP.IsValid() && address.ip.IsValid() && address.ip.IsLoopback() && listenIP.Is4() == address.ip.Is4() && (listenIP.IsUnspecified() || listenIP == address.ip)
		self := meta.Enabled && listenPort == address.port && covered
		ambiguous := meta.Enabled && listenPort == address.port && local && !covered && (listenIP.IsUnspecified() || listenHost == "" || strings.EqualFold(listenHost, "localhost"))
		if declared == "self" {
			if !meta.Enabled || !address.ip.IsLoopback() {
				return "", nil, fmt.Errorf("%s (%s): self DNS declaration requires an enabled original DNS and a literal loopback target", target.field, origin.Source)
			}
			self = true
		} else if declared == "independent" {
			self, ambiguous = false, false
		}
		if ambiguous && declared == "" {
			return "", nil, fmt.Errorf("%s (%s) target %q is ambiguous against original DNS listener %q; declare self or independent in %s", target.field, origin.Source, value, meta.Listen, dnsServicesKey)
		}
		if self {
			if !address.simple {
				return "", nil, fmt.Errorf("%s (%s) self DNS target %q uses an unsupported protocol, alias or URL parameter; use a literal UDP/TCP endpoint", target.field, origin.Source, value)
			}
			if target.purpose != "proxy-server-nameserver" && target.purpose != "proxy-server-nameserver-policy" {
				return "", nil, fmt.Errorf("%s (%s) cannot reference the same DNS service: recursive or unsupported resolver dependency", target.field, origin.Source)
			}
			if respect := dnsValue(dns, "respect-rules"); respect != nil && respect.Value == "true" {
				return "", nil, fmt.Errorf("%s self DNS with respect-rules may bootstrap through the unresolved proxy; this combination is not supported", target.field)
			}
			ref.Service, ref.Effective = "self", gatewayDNSEndpoint
			if strings.Contains(value, "://") {
				ref.Effective = address.scheme + "://" + gatewayDNSEndpoint
			}
			target.node.Value = ref.Effective
		} else if local && address.port == "1053" && (address.scheme == "udp" || address.scheme == "tcp") {
			return "", nil, fmt.Errorf("%s (%s) independent target %q conflicts with OpenSurge DNS port 1053; restore the original source metadata or choose an independent endpoint", target.field, origin.Source, value)
		}
		references = append(references, ref)
	}
	// An explicitly proxied ordinary resolver can also require the node whose
	// own resolver is this service. Keep unknown interface/proxy URL combinations
	// out of the supported self-reference graph, rather than guessing a route.
	hasSelf := false
	for _, ref := range references {
		hasSelf = hasSelf || ref.Service == "self"
	}
	if hasSelf {
		for _, ref := range references {
			if ref.Service == "self" {
				continue
			}
			if strings.Contains(ref.Target, "#") && !strings.HasSuffix(ref.Target, "#DIRECT") {
				return "", nil, fmt.Errorf("%s uses an unverified proxy/interface DNS parameter alongside a self reference; cannot establish bootstrap independence", ref.Field)
			}
		}
	}
	rendered, err := encodeYAMLNode(dns)
	return rendered, references, err
}

// DNSReferenceBindings reads only generated final-config diagnostics. Each
// binding is rechecked against the actual target before preparation uses it.
func DNSReferenceBindings(rendered string) ([]DNSReference, error) {
	root, err := decodeSingleYAMLMapping([]byte(rendered))
	if err != nil {
		return nil, err
	}
	var references []DNSReference
	if node := dnsValue(root, dnsBindingsKey); node != nil {
		if err := decodeDNSMetadata(node, &references); err != nil {
			return nil, err
		}
	}
	return references, nil
}

func hasSelfDNSReference(refs []DNSReference) bool {
	for _, ref := range refs {
		if ref.Service == "self" {
			return true
		}
	}
	return false
}

func finalDNSProbeName(rendered string) (string, error) {
	refs, err := DNSReferenceBindings(rendered)
	if err != nil || !hasSelfDNSReference(refs) {
		return "", err
	}
	root, err := decodeSingleYAMLMapping([]byte(rendered))
	if err != nil {
		return "", err
	}
	name, ok := scalarStringValue(dnsValue(root, dnsProbeKey))
	if !ok || !strings.HasPrefix(name, "opensurge-ready-") || !strings.HasSuffix(name, ".invalid") {
		return "", fmt.Errorf("self DNS configuration lacks a managed readiness probe")
	}
	return name, nil
}

// Legacy overlaid profiles discarded the listener. The Control Service must
// recover their selected raw snapshot before reuse, even if the overlay digest
// has not changed. A missing snapshot cannot be replaced with port heuristics.
func NeedsDNSSourceRecovery(data []byte) bool {
	root, err := decodeSingleYAMLMapping(data)
	if err != nil || dnsValue(root, dnsSourceMetadataKey) != nil {
		return false
	}
	targets, err := dnsTargets(dnsValue(root, "dns"))
	if err != nil {
		return false
	}
	for _, target := range targets {
		address := parseDNSAddress(target.node.Value)
		if address.ip.IsLoopback() || strings.EqualFold(address.host, "localhost") {
			return true
		}
	}
	return false
}
