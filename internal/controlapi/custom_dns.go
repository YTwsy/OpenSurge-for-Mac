package controlapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/mihomo"
)

var errCustomDNSRevision = errors.New("custom DNS changed while it was being edited; reload before saving")

type CustomDNSResponse struct {
	SchemaVersion int                      `json:"schema_version"`
	Revision      string                   `json:"revision"`
	Document      mihomo.CustomDNSDocument `json:"document"`
	Desired       bool                     `json:"desired"`
	Applied       bool                     `json:"applied"`
	Hosts         int                      `json:"hosts"`
	Policies      int                      `json:"policies"`
}

type CustomDNSPreview struct {
	Changes         []mihomo.CustomDNSChange `json:"changes"`
	Warnings        []string                 `json:"warnings"`
	Blockers        []string                 `json:"blockers"`
	FinalYAML       string                   `json:"final_yaml"`
	BaseYAML        string                   `json:"base_yaml"`
	OverlayYAML     string                   `json:"overlay_yaml"`
	CustomYAML      string                   `json:"custom_yaml"`
	ManagedChanges  []mihomo.CustomDNSChange `json:"managed_changes"`
	MacReady        bool                     `json:"mac_ready"`
	DownstreamReady bool                     `json:"downstream_ready"`
	DNSReferences   []mihomo.DNSReference    `json:"dns_references"`
}

func (s *Store) customDNSLocked() (mihomo.CustomDNSDocument, string, error) {
	doc := mihomo.DefaultCustomDNSDocument()
	data, err := os.ReadFile(filepath.Join(s.dir, "custom-dns.json"))
	if err == nil {
		err = json.Unmarshal(data, &doc)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return doc, "", err
	}
	doc, err = mihomo.NormalizeCustomDNS(doc)
	if err != nil {
		return doc, "", err
	}
	data, err = json.Marshal(doc)
	return doc, mihomo.ProfileOverlayDigest(data), err
}

func (s *Server) loadCustomDNS() (mihomo.CustomDNSDocument, string, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	return s.store.customDNSLocked()
}

func (s *Store) saveCustomDNS(doc mihomo.CustomDNSDocument, revision string) error {
	doc, err := mihomo.NormalizeCustomDNS(doc)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, current, err := s.customDNSLocked()
	if err != nil {
		return err
	}
	if revision == "" || revision != current {
		return errCustomDNSRevision
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.dir, "custom-dns.json"), append(data, '\n'), 0o600)
}

func (s *Server) customDNSResponse() (CustomDNSResponse, error) {
	doc, revision, err := s.loadCustomDNS()
	if err != nil {
		return CustomDNSResponse{}, err
	}
	hosts, policies := mihomo.CustomDNSCounts(doc)
	response := CustomDNSResponse{SchemaVersion: SchemaVersion, Revision: revision, Document: doc, Hosts: hosts, Policies: policies}
	cfg, err := config.LoadRuntime(s.configPath)
	if err == nil {
		response.Desired = cfg.Mihomo.ProfileCustomDNSDigest == mihomo.CustomDNSDigest(doc)
		if response.Desired {
			digest, e := config.MihomoProfileDigest(cfg)
			if state, exists := currentBootRuntimeState(cfg); exists && e == nil {
				response.Applied = state.ProfileDigest == digest
			}
		}
	}
	return response, nil
}

func (s *Server) handleCustomDNS(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var doc mihomo.CustomDNSDocument
		if err := decodeJSON(r, &doc, 14<<20); err != nil {
			writeError(w, http.StatusBadRequest, "custom_dns_invalid", err.Error())
			return
		}
		normalized, err := mihomo.NormalizeCustomDNS(doc)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "custom_dns_invalid", err.Error())
			return
		}
		if err = s.store.saveCustomDNS(normalized, strings.Trim(r.Header.Get("If-Match"), `"`)); err != nil {
			status := http.StatusInternalServerError
			code := "custom_dns_save_failed"
			if errors.Is(err, errCustomDNSRevision) {
				status = http.StatusConflict
				code = "revision_conflict"
			}
			writeError(w, status, code, err.Error())
			return
		}
	}
	response, err := s.customDNSResponse()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "custom_dns_failed", err.Error())
		return
	}
	w.Header().Set("ETag", `"`+response.Revision+`"`)
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleCustomDNSImport(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Document mihomo.CustomDNSDocument `json:"document"`
		Format   string                   `json:"format"`
		Content  string                   `json:"content"`
	}
	if err := decodeJSON(r, &request, 26<<20); err != nil {
		writeError(w, http.StatusBadRequest, "custom_dns_import_invalid", err.Error())
		return
	}
	doc, sections, err := mihomo.ImportCustomDNS(request.Document, request.Format, []byte(request.Content))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "custom_dns_import_invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Document mihomo.CustomDNSDocument `json:"document"`
		Sections []string                 `json:"sections"`
	}{doc, sections})
}

func (s *Server) handleCustomDNSPreview(w http.ResponseWriter, r *http.Request) {
	var doc mihomo.CustomDNSDocument
	if err := decodeJSON(r, &doc, 14<<20); err != nil {
		writeError(w, http.StatusBadRequest, "custom_dns_invalid", err.Error())
		return
	}
	doc, err := mihomo.NormalizeCustomDNS(doc)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "custom_dns_invalid", err.Error())
		return
	}
	preview := CustomDNSPreview{Changes: []mihomo.CustomDNSChange{}, Warnings: []string{}, Blockers: []string{}, ManagedChanges: []mihomo.CustomDNSChange{}, DNSReferences: []mihomo.DNSReference{}, CustomYAML: doc.YAML}
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "custom_dns_preview_failed", err.Error())
		return
	}
	preview.MacReady = cfg.ManageSystemDNS()
	preview.DownstreamReady = strings.TrimSpace(cfg.DNS.Upstream) == "" || cfg.DNS.Upstream == config.MihomoDNSUpstream
	input, err := s.policyWorkspaceDraftInput(PolicyWorkspaceRequest{Action: "read"}, true)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "custom_dns_preview_failed", err.Error())
		return
	}
	input.CustomDNS = &doc
	preview.OverlayYAML = mihomo.CustomDNSProfileSection(input.Overlay)
	temporary, err := os.MkdirTemp(s.store.Dir(), ".custom-dns-preview-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "custom_dns_preview_failed", err.Error())
		return
	}
	defer os.RemoveAll(temporary)
	composition := mihomo.CustomDNSComposition{}
	candidate, base, err := materializeWorkspaceCandidate(filepath.Join(temporary, "config.yaml"), cfg, input, &composition)
	if err != nil {
		preview.Blockers = append(preview.Blockers, err.Error())
		writeJSON(w, http.StatusOK, preview)
		return
	}
	if base != nil {
		preview.BaseYAML = mihomo.CustomDNSProfileSection(base.Source)
	} else if cfg.Mihomo.ProfileMode == config.MihomoProfileModeManaged {
		if raw, e := mihomo.RenderManagedBaseProfile(cfg); e == nil {
			preview.BaseYAML = mihomo.CustomDNSProfileSection([]byte(raw))
		}
	} else if cfg.Mihomo.ProfileOverlayDigest == "" && cfg.Mihomo.ProfileCustomDNSDigest == "" {
		if raw, e := os.ReadFile(cfg.Mihomo.Profile); e == nil {
			preview.BaseYAML = mihomo.CustomDNSProfileSection(raw)
		}
	}
	preview.Changes, preview.Warnings = composition.Changes, composition.Warnings
	if preview.Changes == nil {
		preview.Changes = []mihomo.CustomDNSChange{}
	}
	if preview.Warnings == nil {
		preview.Warnings = []string{}
	}
	if candidate.Mihomo.ProfileMode == config.MihomoProfileModeImported {
		payload, e := os.ReadFile(candidate.Mihomo.Profile)
		if e != nil {
			preview.Blockers = append(preview.Blockers, e.Error())
		} else {
			preview.FinalYAML = mihomo.CustomDNSProfileSection(payload)
			if e = mihomo.ValidateCustomDNSCandidate(candidate, payload); e != nil {
				preview.Blockers = append(preview.Blockers, e.Error())
			}
		}
	}
	// Render uses the same DNS coordination/reference binding as a real candidate.
	// When an entry prerequisite fails, the uncoordinated result above remains
	// visible and is explicitly blocked rather than presented as final/applied.
	final, err := mihomo.RenderConfig(candidate)
	if err != nil {
		if len(preview.Blockers) == 0 {
			preview.Blockers = append(preview.Blockers, err.Error())
		}
	} else {
		preview.ManagedChanges = mihomo.CustomDNSSectionChanges([]byte(preview.FinalYAML), []byte(final), "managed")
		preview.FinalYAML = mihomo.CustomDNSProfileSection([]byte(final))
		preview.DNSReferences, err = mihomo.DNSReferenceBindings(final)
		if preview.DNSReferences == nil {
			preview.DNSReferences = []mihomo.DNSReference{}
		}
		if err != nil {
			preview.Blockers = append(preview.Blockers, err.Error())
		}
	}
	if doc.Enabled && len(cfg.LocalSystemDNS.Resolvers) == 0 {
		preview.Warnings = append(preview.Warnings, "system resolver snapshot is not yet available; startup may supplement private-domain policies from the captured system DNS")
	}
	if doc.Enabled {
		preview.Warnings = append(preview.Warnings, "configuration preview only; actual Mac TUN and downstream DNS queries have not been tested")
	}
	writeJSON(w, http.StatusOK, preview)
}

// composeDNSDraft keeps source selection/preview and workspace startup on the
// same ordering. Snapshot inputs are read once per operation.
func (s *Server) composeDNSDraft(source []byte, overlay mihomo.ProfileOverlayDocument) (mihomo.ProfileOverlayComposition, error) {
	composed, err := mihomo.ComposeProfileOverlay(source, overlay)
	if err != nil {
		return composed, err
	}
	doc, _, err := s.loadCustomDNS()
	if err != nil {
		return composed, fmt.Errorf("custom DNS: %w", err)
	}
	custom, err := mihomo.ComposeCustomDNS([]byte(composed.ProfileYAML), doc)
	return custom.ProfileOverlayComposition, err
}
