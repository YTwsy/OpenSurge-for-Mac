package gatewayrules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingReturnsStableEmptyRevision(t *testing.T) {
	document, revision, err := Load(filepath.Join(t.TempDir(), "gateway-rules.json"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if document.SchemaVersion != CurrentSchemaVersion || len(document.Prepend) != 0 || len(document.Append) != 0 || len(document.Delete) != 0 {
		t.Fatalf("unexpected empty document: %#v", document)
	}
	if revision != Digest(document) || revision == "" {
		t.Fatalf("revision = %q, digest = %q", revision, Digest(document))
	}
}

func TestLoadNormalizesAndDigestsDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway-rules.json")
	if err := os.WriteFile(path, []byte(`{"prepend":[" DOMAIN,example.com,Proxy "],"append":["MATCH,DIRECT"],"delete":[]}`), 0o640); err != nil {
		t.Fatal(err)
	}
	document, revision, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if document.SchemaVersion != CurrentSchemaVersion || document.Prepend[0] != "DOMAIN,example.com,Proxy" {
		t.Fatalf("document was not normalized: %#v", document)
	}
	if revision != Digest(document) {
		t.Fatalf("revision = %q, digest = %q", revision, Digest(document))
	}
}

func TestValidateRejectsUnsupportedSchemaAndEmptyRules(t *testing.T) {
	if err := Validate(Document{SchemaVersion: 2}); err == nil {
		t.Fatal("Validate() accepted unsupported schema")
	}
	if err := Validate(Document{Prepend: []string{"   "}}); err == nil {
		t.Fatal("Validate() accepted an empty rule")
	}
	if err := Validate(Normalize(Document{Prepend: []string{"MATCH,DIRECT\nexternal-controller: 0.0.0.0:9090"}})); err == nil {
		t.Fatal("Validate() accepted a multiline rule")
	}
	if err := Validate(Normalize(Document{Append: []string{"DOMAIN,example.com,\tDIRECT"}})); err == nil {
		t.Fatal("Validate() accepted a control character")
	}
	for _, separator := range []string{"\u2028", "\u2029"} {
		if err := Validate(Normalize(Document{Append: []string{"DOMAIN,example.com," + separator + "DIRECT"}})); err == nil {
			t.Fatalf("Validate() accepted Unicode line separator %U", []rune(separator)[0])
		}
	}
}
