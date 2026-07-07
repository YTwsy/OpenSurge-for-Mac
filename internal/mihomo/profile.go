package mihomo

import (
	"fmt"
	"os"
	"strings"
)

var importableProfileSections = map[string]bool{
	"proxies":         true,
	"proxy-providers": true,
	"proxy-groups":    true,
	"rule-providers":  true,
	"rules":           true,
}

func LoadImportedProfileSections(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read imported mihomo profile: %w", err)
	}

	blocks, found := extractImportableProfileSections(string(data))
	if !found["rules"] {
		return "", fmt.Errorf("imported mihomo profile must contain a top-level rules section")
	}
	if len(blocks) == 0 {
		return "", fmt.Errorf("imported mihomo profile contains no importable sections")
	}

	return strings.TrimRight(strings.Join(blocks, "\n"), "\n") + "\n", nil
}

func extractImportableProfileSections(profile string) ([]string, map[string]bool) {
	lines := strings.SplitAfter(profile, "\n")
	found := make(map[string]bool)
	var blocks []string
	var current strings.Builder
	collecting := false

	flush := func() {
		if !collecting {
			return
		}
		block := strings.TrimRight(current.String(), "\n")
		if block != "" {
			blocks = append(blocks, block)
		}
		current.Reset()
		collecting = false
	}

	for _, line := range lines {
		if key, ok := topLevelYAMLKey(line); ok {
			if importableProfileSections[key] {
				flush()
				collecting = true
				found[key] = true
				current.WriteString(line)
				continue
			}
			flush()
			continue
		}
		if collecting {
			current.WriteString(line)
		}
	}
	flush()

	return blocks, found
}

func topLevelYAMLKey(line string) (string, bool) {
	if line == "" || line[0] == ' ' || line[0] == '\t' {
		return "", false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "---" || trimmed == "..." {
		return "", false
	}
	key, _, ok := strings.Cut(trimmed, ":")
	if !ok {
		return "", false
	}
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, " \t\"'{}[]") {
		return "", false
	}
	return key, true
}
