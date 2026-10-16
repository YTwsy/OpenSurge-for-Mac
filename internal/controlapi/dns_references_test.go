package controlapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/mihomo"
)

const dnsReferenceSource = `dns:
  enable: true
  listen: 0.0.0.0:7874
  nameserver: [192.0.2.53]
  proxy-server-nameserver: [udp://127.0.0.1:7874]
rules: ['MATCH,DIRECT']
`

func TestDNSReferencePreviewWithoutOverlay(t *testing.T) {
	server := newTestServer(t)
	source, err := server.importReader("dns", "mihomo_profile", "file:dns.yaml", strings.NewReader(dnsReferenceSource))
	if err != nil {
		t.Fatal(err)
	}
	response := performAuthorized(server, http.MethodGet, "/api/v1/sources/"+source.ID+"/preview", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", response.Code, response.Body)
	}
	var preview ProfileOverlayPreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.SourceYAML != dnsReferenceSource || preview.EffectiveProfileYAML != dnsReferenceSource {
		t.Fatal("preview changed the disabled-overlay source")
	}
	if len(preview.DNSReferences) != 2 {
		t.Fatalf("missing resolver diagnostics: %+v", preview.DNSReferences)
	}
	ref := preview.DNSReferences[1]
	if ref.Target != "udp://127.0.0.1:7874" || ref.Effective != "udp://127.0.0.1:1053" || ref.Service != "self" || ref.Source != "subscription" || ref.SourceDigest != source.Digest {
		t.Fatalf("wrong DNS reference: %+v", ref)
	}
	overlay := []byte("schema-version: 1\nenabled: true\ndns:\n  merge:\n    nameserver: [127.0.0.1:7874]\n")
	if err := server.store.SaveProfileOverlay(overlay); err != nil {
		t.Fatal(err)
	}
	response = performAuthorized(server, http.MethodGet, "/api/v1/sources/"+source.ID+"/preview", nil)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "dns.nameserver[0]") || !strings.Contains(response.Body.String(), "global-overlay") {
		t.Fatalf("missing precise conflict diagnostic: %d %s", response.Code, response.Body)
	}
}

func TestLegacyDNSReferencesRecoverRawSourceEvenWhenOverlayUnchanged(t *testing.T) {
	server := newTestServer(t)
	source, err := server.importReader("dns", "mihomo_profile", "file:dns.yaml", strings.NewReader(dnsReferenceSource))
	if err != nil {
		t.Fatal(err)
	}
	overlay := []byte("schema-version: 1\nenabled: true\n")
	if err := server.store.SaveProfileOverlay(overlay); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = filepath.Join(filepath.Dir(server.configPath), "legacy.yaml")
	cfg.Mihomo.ProfileSourceDigest = source.Digest
	cfg.Mihomo.ProfileOverlayDigest = mihomo.ProfileOverlayDigest(overlay)
	legacy := "dns:\n  nameserver: [192.0.2.53]\n  proxy-server-nameserver: [udp://127.0.0.1:7874]\nrules: ['MATCH,DIRECT']\n"
	if err := writeAtomic(cfg.Mihomo.Profile, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(server.configPath, []byte(config.Render(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mihomo.RenderConfig(cfg); err == nil || !strings.Contains(err.Error(), "original DNS listener metadata") {
		t.Fatalf("legacy render guessed a target: %v", err)
	}
	input, err := server.policyWorkspaceInput(PolicyWorkspaceRequest{Action: "read"})
	if err != nil || string(input.Source) != dnsReferenceSource {
		t.Fatalf("unchanged overlay bypassed DNS recovery: %v", err)
	}
	candidate, base, err := workspaceCandidate(server.configPath, cfg, input)
	if err != nil || base == nil || string(base.Source) != dnsReferenceSource {
		t.Fatalf("raw source not preserved across Helper candidate: %v", err)
	}
	final, err := mihomo.RenderConfig(candidate)
	if err != nil {
		t.Fatal(err)
	}
	refs, _ := mihomo.DNSReferenceBindings(final)
	if len(refs) != 2 || refs[1].Service != "self" {
		t.Fatalf("legacy DNS recovery failed: %+v", refs)
	}
	input.Source = nil
	if _, _, err := workspaceCandidate(server.configPath, cfg, input); err == nil || !strings.Contains(err.Error(), "raw source are unavailable") {
		t.Fatalf("missing source was guessed: %v", err)
	}
}
