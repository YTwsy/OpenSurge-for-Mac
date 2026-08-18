// Package gatewayrules stores the user-owned rule overlay that is applied to
// the active mihomo profile. It intentionally lives outside the imported
// subscription YAML so a refresh cannot replace it.
package gatewayrules

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

const CurrentSchemaVersion = 1

// Document mirrors the useful part of Clash Verge's profile enhancement
// sequence: prepend and append rules are user-owned, while delete contains
// exact source-rule values that should be omitted when the profile is merged.
type Document struct {
	SchemaVersion int      `json:"schema_version"`
	Prepend       []string `json:"prepend"`
	Append        []string `json:"append"`
	Delete        []string `json:"delete"`
}

func Default() Document {
	return Document{
		SchemaVersion: CurrentSchemaVersion,
		Prepend:       []string{},
		Append:        []string{},
		Delete:        []string{},
	}
}

func Load(path string) (Document, string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		document := Default()
		return document, Digest(document), nil
	}
	if err != nil {
		return Document{}, "", err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, "", fmt.Errorf("decode gateway rules: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Document{}, "", fmt.Errorf("decode gateway rules: multiple JSON values")
		}
		return Document{}, "", fmt.Errorf("decode gateway rules: %w", err)
	}
	if err := Validate(document); err != nil {
		return Document{}, "", err
	}
	document = Normalize(document)
	return document, Digest(document), nil
}

func Normalize(document Document) Document {
	if document.SchemaVersion == 0 {
		document.SchemaVersion = CurrentSchemaVersion
	}
	document.Prepend = normalizeRules(document.Prepend)
	document.Append = normalizeRules(document.Append)
	document.Delete = normalizeRules(document.Delete)
	return document
}

func Validate(document Document) error {
	if document.SchemaVersion != 0 && document.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported gateway rules schema version %d", document.SchemaVersion)
	}
	for name, rules := range map[string][]string{
		"prepend": document.Prepend,
		"append":  document.Append,
		"delete":  document.Delete,
	} {
		for index, rule := range rules {
			if strings.TrimSpace(rule) == "" {
				return fmt.Errorf("gateway rules %s[%d] must not be empty", name, index)
			}
			if strings.IndexFunc(rule, func(value rune) bool {
				return unicode.IsControl(value) || unicode.In(value, unicode.Zl, unicode.Zp)
			}) >= 0 {
				return fmt.Errorf("gateway rules %s[%d] must be a single line without control characters or Unicode line separators", name, index)
			}
		}
	}
	return nil
}

func Canonical(document Document) ([]byte, error) {
	document = Normalize(document)
	if err := Validate(document); err != nil {
		return nil, err
	}
	return json.Marshal(document)
}

func Digest(document Document) string {
	data, err := Canonical(document)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func normalizeRules(rules []string) []string {
	if len(rules) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(rules))
	for _, rule := range rules {
		result = append(result, strings.TrimSpace(rule))
	}
	return result
}
