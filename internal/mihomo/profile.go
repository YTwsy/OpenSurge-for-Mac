package mihomo

import (
	"fmt"
	"os"
	"path/filepath"
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
	blocks, err := loadImportedProfileBlocks(path)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(renderImportedProfileBlocks(blocks, ""), "\n") + "\n", nil
}

func loadImportedProfileBlocks(path string) ([]importedProfileBlock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read imported mihomo profile: %w", err)
	}

	blocks, found := extractImportableProfileSections(string(data))
	if !found["rules"] {
		return nil, fmt.Errorf("imported mihomo profile must contain a top-level rules section")
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("imported mihomo profile contains no importable sections")
	}
	profileDir := filepath.Dir(path)
	for i, block := range blocks {
		if block.key == "proxy-providers" || block.key == "rule-providers" {
			blocks[i].text = rewriteProviderPaths(block.text, profileDir)
		}
	}
	return blocks, nil
}

type importedProfileBlock struct {
	key  string
	text string
}

func extractImportableProfileSections(profile string) ([]importedProfileBlock, map[string]bool) {
	lines := strings.SplitAfter(profile, "\n")
	found := make(map[string]bool)
	var blocks []importedProfileBlock
	var current strings.Builder
	currentKey := ""
	collecting := false

	flush := func() {
		if !collecting {
			return
		}
		block := strings.TrimRight(current.String(), "\n")
		if block != "" {
			blocks = append(blocks, importedProfileBlock{key: currentKey, text: block})
		}
		current.Reset()
		currentKey = ""
		collecting = false
	}

	for _, line := range lines {
		if key, ok := topLevelYAMLKey(line); ok {
			if importableProfileSections[key] {
				flush()
				collecting = true
				currentKey = key
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

func renderImportedProfileBlocks(blocks []importedProfileBlock, profileDir string) string {
	rendered := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if profileDir != "" && (block.key == "proxy-providers" || block.key == "rule-providers") {
			rendered = append(rendered, rewriteProviderPaths(block.text, profileDir))
			continue
		}
		rendered = append(rendered, block.text)
	}
	return strings.Join(rendered, "\n")
}

func rewriteProviderPaths(block string, profileDir string) string {
	lines := strings.SplitAfter(block, "\n")
	for i, line := range lines {
		lines[i] = rewriteProviderPathLine(line, profileDir)
	}
	return strings.Join(lines, "")
}

func rewriteProviderPathLine(line string, profileDir string) string {
	indentLen := len(line) - len(strings.TrimLeft(line, " \t"))
	indent := line[:indentLen]
	body := line[indentLen:]
	trimmedBody := strings.TrimSpace(body)
	if trimmedBody == "" || strings.HasPrefix(trimmedBody, "#") {
		return line
	}

	key, rest, ok := strings.Cut(body, ":")
	if !ok || strings.TrimSpace(key) != "path" {
		return line
	}

	lineEnding := ""
	if strings.HasSuffix(rest, "\n") {
		lineEnding = "\n"
		rest = strings.TrimSuffix(rest, "\n")
	}
	valuePart, commentPart := splitInlineComment(rest)
	prefixWhitespace := leadingWhitespace(valuePart)
	commentWhitespace := trailingWhitespace(valuePart)
	value := strings.TrimSpace(valuePart)
	quote := pathQuote(value)
	unquoted := strings.Trim(value, `"'`)
	if !relativeProviderPath(unquoted) {
		return line
	}

	rewritten := filepath.Join(profileDir, unquoted)
	return indent + key + ":" + prefixWhitespace + quote + rewritten + quote + commentWhitespace + commentPart + lineEnding
}

func splitInlineComment(value string) (string, string) {
	inSingle := false
	inDouble := false
	for i, r := range value {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return value[:i], value[i:]
			}
		}
	}
	return value, ""
}

func leadingWhitespace(value string) string {
	return value[:len(value)-len(strings.TrimLeft(value, " \t"))]
}

func trailingWhitespace(value string) string {
	return value[len(strings.TrimRight(value, " \t")):]
}

func pathQuote(value string) string {
	if len(value) >= 2 {
		switch {
		case value[0] == '"' && value[len(value)-1] == '"':
			return `"`
		case value[0] == '\'' && value[len(value)-1] == '\'':
			return `'`
		}
	}
	return ""
}

func relativeProviderPath(value string) bool {
	if value == "" || filepath.IsAbs(value) {
		return false
	}
	if strings.Contains(value, "://") {
		return false
	}
	return true
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
