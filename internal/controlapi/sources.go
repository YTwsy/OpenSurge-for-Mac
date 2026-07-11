package controlapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const maxSourceSize = 10 << 20

func (s *Server) importURL(ctx context.Context, req SourceImportRequest) (Source, error) {
	parsed, err := url.Parse(req.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return Source{}, fmt.Errorf("source URL must be an absolute HTTPS URL")
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			if next.URL.Scheme != "https" {
				return fmt.Errorf("redirected source must remain HTTPS")
			}
			return nil
		},
		Transport: &http.Transport{DialContext: safeDialContext},
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return Source{}, err
	}
	httpReq.Header.Set("User-Agent", "OpenSurge-for-Mac/1")
	resp, err := client.Do(httpReq)
	if err != nil {
		return Source{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Source{}, fmt.Errorf("source returned %s", resp.Status)
	}
	before, _ := s.store.Sources()
	source, err := s.importReader(req.Name, req.Kind, redactURL(req.URL), io.LimitReader(resp.Body, maxSourceSize+1))
	if err != nil {
		return Source{}, err
	}
	if err := s.credentials.Put(ctx, source.ID, req.URL); err != nil {
		_ = s.store.SaveSources(before)
		_ = os.Remove(source.SnapshotPath)
		return Source{}, err
	}
	return source, nil
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return nil, fmt.Errorf("source URL resolves to a private or local address")
		}
	}
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
}

func redactURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return "invalid-url"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func (s *Server) importReader(name, kind, origin string, reader io.Reader) (Source, error) {
	if strings.TrimSpace(name) == "" {
		name = "Imported profile"
	}
	if kind == "" {
		kind = "mihomo_profile"
	}
	if kind != "mihomo_profile" && kind != "rule_provider" {
		return Source{}, fmt.Errorf("unsupported source kind %q", kind)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return Source{}, err
	}
	if len(data) == 0 {
		return Source{}, fmt.Errorf("source is empty")
	}
	if len(data) > maxSourceSize {
		return Source{}, fmt.Errorf("source exceeds 10 MiB limit")
	}
	inventory, err := inspectSource(data, kind)
	valid := err == nil
	validation := "structural validation passed; apply runs mihomo engine validation"
	if err != nil {
		validation = err.Error()
	}
	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	idBytes := sha256.Sum256([]byte(name + "\x00" + origin))
	id := hex.EncodeToString(idBytes[:8])
	dir := filepath.Join(s.store.Dir(), "sources", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Source{}, err
	}
	path := filepath.Join(dir, digest+".yaml")
	if err := writeAtomic(path, data, 0o600); err != nil {
		return Source{}, err
	}
	source := Source{
		SchemaVersion: SchemaVersion,
		ID:            id,
		Name:          name,
		Kind:          kind,
		Origin:        origin,
		SnapshotPath:  path,
		Digest:        digest,
		Size:          int64(len(data)),
		Valid:         valid,
		Validation:    validation,
		Inventory:     inventory,
		ImportedAt:    time.Now().UTC(),
		Versions:      []SourceVersion{},
		Diff:          emptySourceDiff(),
	}
	if strings.HasPrefix(origin, "https://") {
		source.Origin = redactURL(origin)
	}
	sources, loadErr := s.store.Sources()
	if loadErr != nil {
		return Source{}, loadErr
	}
	for i := range sources {
		if sources[i].ID == source.ID {
			previous := sources[i]
			if previous.Digest == source.Digest {
				source.Versions = append([]SourceVersion{}, previous.Versions...)
				source.Applied = previous.Applied
				source.Diff.PreviousDigest = previous.Digest
			} else {
				source.Versions = append(append([]SourceVersion{}, previous.Versions...), sourceVersion(previous))
				source.Diff = diffInventory(previous.Digest, previous.Inventory, source.Inventory)
			}
			sources[i] = source
			return source, s.store.SaveSources(sources)
		}
	}
	sources = append(sources, source)
	return source, s.store.SaveSources(sources)
}

func sourceVersion(source Source) SourceVersion {
	return SourceVersion{Digest: source.Digest, Size: source.Size, Valid: source.Valid, Validation: source.Validation, Inventory: source.Inventory, ImportedAt: source.ImportedAt, Applied: source.Applied, SnapshotPath: source.SnapshotPath}
}

func emptySourceDiff() SourceDiff {
	return SourceDiff{ProxiesAdded: []string{}, ProxiesRemoved: []string{}, GroupsAdded: []string{}, GroupsRemoved: []string{}, ProxyProvidersAdded: []string{}, ProxyProvidersRemoved: []string{}, RuleProvidersAdded: []string{}, RuleProvidersRemoved: []string{}}
}

func diffInventory(previousDigest string, before, after Inventory) SourceDiff {
	diff := emptySourceDiff()
	diff.PreviousDigest = previousDigest
	diff.ProxiesAdded, diff.ProxiesRemoved = diffNames(before.Proxies, after.Proxies)
	diff.GroupsAdded, diff.GroupsRemoved = diffNames(before.ProxyGroups, after.ProxyGroups)
	diff.ProxyProvidersAdded, diff.ProxyProvidersRemoved = diffNames(before.ProxyProviders, after.ProxyProviders)
	diff.RuleProvidersAdded, diff.RuleProvidersRemoved = diffNames(before.RuleProviders, after.RuleProviders)
	diff.RuleCountDelta = after.RuleCount - before.RuleCount
	return diff
}

func diffNames(before, after []string) (added, removed []string) {
	left, right := map[string]bool{}, map[string]bool{}
	for _, value := range before {
		left[value] = true
	}
	for _, value := range after {
		right[value] = true
	}
	for value := range right {
		if !left[value] {
			added = append(added, value)
		}
	}
	for value := range left {
		if !right[value] {
			removed = append(removed, value)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	if added == nil {
		added = []string{}
	}
	if removed == nil {
		removed = []string{}
	}
	return added, removed
}

func inspectSource(data []byte, kind string) (Inventory, error) {
	var document yaml.Node
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&document); err != nil {
		return Inventory{}, fmt.Errorf("parse YAML: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return Inventory{}, fmt.Errorf("top-level YAML must be a mapping")
	}
	root := document.Content[0]
	sections := map[string]*yaml.Node{}
	for i := 0; i < len(root.Content); i += 2 {
		key := root.Content[i]
		if _, exists := sections[key.Value]; exists {
			return Inventory{}, fmt.Errorf("duplicate top-level section %q", key.Value)
		}
		sections[key.Value] = root.Content[i+1]
	}
	inv := Inventory{
		Proxies:        sequenceNames(sections["proxies"]),
		ProxyGroups:    sequenceNames(sections["proxy-groups"]),
		ProxyProviders: mappingKeys(sections["proxy-providers"]),
		RuleProviders:  mappingKeys(sections["rule-providers"]),
	}
	if kind == "mihomo_profile" {
		rules := sections["rules"]
		if rules == nil || rules.Kind != yaml.SequenceNode {
			return inv, fmt.Errorf("mihomo profile requires a top-level rules sequence")
		}
		inv.RuleCount = len(rules.Content)
		if inv.RuleCount > 0 {
			last := rules.Content[inv.RuleCount-1]
			inv.TerminalMatch = last.Kind == yaml.ScalarNode && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(last.Value)), "MATCH,")
		}
		if !inv.TerminalMatch {
			inv.Warnings = append(inv.Warnings, "rules do not end in terminal MATCH")
		}
	}
	for _, name := range append(append([]string{}, inv.ProxyGroups...), inv.RuleProviders...) {
		if strings.HasPrefix(name, "device/") || strings.HasPrefix(name, "open-surge-ruleset-") {
			return inv, fmt.Errorf("imported source uses reserved OpenSurge name %q", name)
		}
	}
	return inv, nil
}

func sequenceNames(node *yaml.Node) []string {
	if node == nil || node.Kind != yaml.SequenceNode {
		return []string{}
	}
	var names []string
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i < len(item.Content); i += 2 {
			if item.Content[i].Value == "name" && item.Content[i+1].Kind == yaml.ScalarNode {
				names = append(names, item.Content[i+1].Value)
			}
		}
	}
	sort.Strings(names)
	return names
}

func mappingKeys(node *yaml.Node) []string {
	if node == nil || node.Kind != yaml.MappingNode {
		return []string{}
	}
	keys := make([]string, 0, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		keys = append(keys, node.Content[i].Value)
	}
	sort.Strings(keys)
	return keys
}
