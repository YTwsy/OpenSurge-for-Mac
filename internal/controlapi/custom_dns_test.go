package controlapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/dhcp"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

func customDNSTestDocument() mihomo.CustomDNSDocument {
	return mihomo.CustomDNSDocument{SchemaVersion: 1, Enabled: true, YAML: "hosts: {nas.home.arpa: 192.0.2.8}\ndns: {fake-ip-filter: ['+.corp.example']}\n"}
}

func TestCustomDNSStoreCASAndAPI(t *testing.T) {
	server := newTestServer(t)
	response := performAuthorized(server, http.MethodGet, "/api/v1/custom-dns", nil)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var original CustomDNSResponse
	if err := json.Unmarshal(response.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	if original.Document.Enabled || original.Revision == "" {
		t.Fatalf("unexpected default: %+v", original)
	}
	save := func(doc mihomo.CustomDNSDocument, revision string) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(doc)
		request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:61767/api/v1/custom-dns", bytes.NewReader(payload))
		request.Host = "127.0.0.1:61767"
		request.Header.Set("Authorization", "Bearer "+server.token)
		request.Header.Set("If-Match", `"`+revision+`"`)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		return recorder
	}
	before := fileDigest(server.configPath)
	response = save(customDNSTestDocument(), original.Revision)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	if before != fileDigest(server.configPath) {
		t.Fatal("saving draft modified desired gateway config")
	}
	var saved CustomDNSResponse
	_ = json.Unmarshal(response.Body.Bytes(), &saved)
	if saved.Applied || saved.Desired || saved.Hosts != 1 {
		t.Fatalf("draft marked applied: %+v", saved)
	}
	if response = save(original.Document, original.Revision); response.Code != http.StatusConflict {
		t.Fatalf("stale save accepted: %s", response.Body.String())
	}
	invalid := customDNSTestDocument()
	invalid.YAML = "dns: {listen: ':53'}"
	if response = save(invalid, saved.Revision); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid save accepted: %s", response.Body.String())
	}
	_, revision, err := server.loadCustomDNS()
	if err != nil || revision != saved.Revision {
		t.Fatalf("failed save changed draft: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			doc := customDNSTestDocument()
			doc.YAML = "hosts: {other.example: 192.0.2.9}"
			if n == 1 {
				doc.Enabled = false
			}
			results <- server.store.saveCustomDNS(doc, revision)
		}(i)
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, errCustomDNSRevision) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("CAS success=%d conflicts=%d", success, conflicts)
	}
}

func TestCustomDNSImportAndPreviewDoNotSave(t *testing.T) {
	server := newTestServer(t)
	_, revision, _ := server.loadCustomDNS()
	before := fileDigest(server.configPath)
	payload, _ := json.Marshal(map[string]any{"document": customDNSTestDocument(), "format": "hosts", "content": "192.0.2.10 other.home.arpa\n"})
	response := performAuthorized(server, http.MethodPost, "/api/v1/custom-dns/import", payload)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "other.home.arpa") {
		t.Fatal(response.Body.String())
	}
	var imported struct {
		Document mihomo.CustomDNSDocument `json:"document"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(imported.Document.YAML, "nas.home.arpa") || !strings.Contains(imported.Document.YAML, "corp.example") {
		t.Fatal(imported.Document.YAML)
	}
	payload, _ = json.Marshal(customDNSTestDocument())
	response = performAuthorized(server, http.MethodPost, "/api/v1/custom-dns/preview", payload)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var preview CustomDNSPreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.FinalYAML, "nas.home.arpa") || len(preview.Changes) == 0 {
		t.Fatalf("preview missing composed DNS: %s", response.Body.String())
	}
	_, after, _ := server.loadCustomDNS()
	if after != revision || before != fileDigest(server.configPath) {
		t.Fatal("import/preview modified draft or gateway config")
	}
	entries, _ := filepath.Glob(filepath.Join(server.store.Dir(), ".custom-dns-preview-*"))
	if len(entries) != 0 {
		t.Fatal("preview leaked temporary files")
	}
	bad := customDNSTestDocument()
	bad.YAML = "hosts: {lease.lan: 192.0.2.9}"
	payload, _ = json.Marshal(bad)
	response = performAuthorized(server, http.MethodPost, "/api/v1/custom-dns/preview", payload)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	_ = json.Unmarshal(response.Body.Bytes(), &preview)
	if len(preview.Blockers) == 0 {
		t.Fatal("preview did not report DHCP boundary")
	}
}

func TestCustomDNSWorkspaceEnableDisableAndSnapshot(t *testing.T) {
	for _, hasSource := range []bool{false, true} {
		for _, hasOverlay := range []bool{false, true} {
			name := "managed"
			if hasSource {
				name = "imported"
			}
			if hasOverlay {
				name += "-overlay"
			}
			t.Run(name, func(t *testing.T) {
				path, cfg := workspaceCandidateTestConfig(t)
				cfg.Transparent.Mode = "tun"
				cfg.Transparent.TUNAutoRoute = true
				cfg.LocalSystemDNS.Enabled = true
				raw := []byte("dns: {nameserver: [1.1.1.1]}\nrules: ['MATCH,DIRECT']\n")
				if hasSource {
					cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
					cfg.Mihomo.Profile = filepath.Join(filepath.Dir(path), "source.yaml")
					if err := writeAtomic(cfg.Mihomo.Profile, raw, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				doc := customDNSTestDocument()
				input := PolicyWorkspaceInput{CustomDNS: &doc}
				if hasOverlay {
					input.Overlay = []byte(policyWorkspaceOverlayFixture)
				}
				candidate, base, err := workspaceCandidate(path, cfg, input)
				if err != nil {
					t.Fatal(err)
				}
				if candidate.Mihomo.ProfileCustomDNSDigest == "" || base == nil {
					t.Fatalf("custom-only not materialized: %+v", candidate.Mihomo)
				}
				rendered, err := mihomo.RenderConfig(candidate)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(rendered, "nas.home.arpa") {
					t.Fatal(rendered)
				}
				lan, err := dhcp.RenderConfig(candidate, runtime.NewPaths(candidate))
				if err != nil || !strings.Contains(lan, "no-hosts") {
					t.Fatalf("LAN hosts ownership: %v %s", err, lan)
				}
				data, _ := json.Marshal(base)
				if err := writeAtomic(policyWorkspaceBasePath(candidate), data, 0o600); err != nil {
					t.Fatal(err)
				}
				twice, _, err := workspaceCandidate(path, candidate, input)
				if err != nil || twice.Mihomo.Profile != candidate.Mihomo.Profile {
					t.Fatalf("repeated composition changed: %v", err)
				}
				changed := candidate
				changed.DHCP.Domain = "home.arpa"
				if _, _, err = workspaceCandidate(path, changed, input); err == nil {
					t.Fatal("DHCP namespace change bypassed guard")
				}
				doc.Enabled = false
				disabled, _, err := workspaceCandidate(path, candidate, input)
				if err != nil {
					t.Fatal(err)
				}
				final, err := mihomo.RenderConfig(disabled)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(final, "nas.home.arpa") || disabled.Mihomo.ProfileCustomDNSDigest != "" {
					t.Fatal("disabling did not restore inherited DNS")
				}
				lan, err = dhcp.RenderConfig(disabled, runtime.NewPaths(disabled))
				if err != nil || strings.Contains(lan, "no-hosts") {
					t.Fatal("disabled DNS retained no-hosts")
				}
				if hasSource {
					after, _ := os.ReadFile(cfg.Mihomo.Profile)
					if !bytes.Equal(after, raw) {
						t.Fatal("raw source changed")
					}
				}
			})
		}
	}
}
