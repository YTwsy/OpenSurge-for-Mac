package controlapi

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

const maxProfileOverlaySize = 2 << 20

func (s *Server) handleProfileOverlay(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		response, err := s.profileOverlayResponse()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "profile_overlay_failed", err.Error())
			return
		}
		w.Header().Set("ETag", `"`+response.Revision+`"`)
		writeJSON(w, http.StatusOK, response)
		return
	}

	_, _, currentRevision, err := s.loadProfileOverlay()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "profile_overlay_failed", err.Error())
		return
	}
	match := strings.Trim(r.Header.Get("If-Match"), `"`)
	if match == "" || match != currentRevision {
		writeError(w, http.StatusConflict, "revision_conflict", "global profile overlay changed while it was being edited; reload and try again")
		return
	}
	var request ProfileOverlaySaveRequest
	if err := decodeJSON(r, &request, maxProfileOverlaySize); err != nil {
		writeError(w, http.StatusBadRequest, "profile_overlay_invalid", err.Error())
		return
	}
	if (request.YAML == nil) == (request.Document == nil) {
		writeError(w, http.StatusBadRequest, "profile_overlay_invalid", "provide exactly one of yaml or document")
		return
	}
	var data []byte
	if request.YAML != nil {
		data = []byte(strings.TrimRight(*request.YAML, "\r\n") + "\n")
		if _, err := mihomo.ParseProfileOverlay(data); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "profile_overlay_invalid", err.Error())
			return
		}
	} else {
		data, err = mihomo.RenderProfileOverlay(*request.Document)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "profile_overlay_invalid", err.Error())
			return
		}
	}
	if len(data) > maxProfileOverlaySize {
		writeError(w, http.StatusRequestEntityTooLarge, "profile_overlay_too_large", "global profile overlay exceeds 2 MiB")
		return
	}
	if err := s.store.SaveProfileOverlay(data); err != nil {
		writeError(w, http.StatusInternalServerError, "profile_overlay_save_failed", err.Error())
		return
	}
	response, err := s.profileOverlayResponse()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "profile_overlay_failed", err.Error())
		return
	}
	w.Header().Set("ETag", `"`+response.Revision+`"`)
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleSourcePreview(w http.ResponseWriter, r *http.Request) {
	source, err := s.sourceByID(r.PathValue("id"))
	if err != nil || !source.Valid || source.Kind != "mihomo_profile" {
		writeError(w, http.StatusConflict, "source_not_previewable", "source must be a structurally valid mihomo profile")
		return
	}
	sourceData, err := os.ReadFile(source.SnapshotPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "source_snapshot_unavailable", err.Error())
		return
	}
	overlayData, document, _, err := s.loadProfileOverlay()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "profile_overlay_failed", err.Error())
		return
	}
	composition, err := mihomo.ComposeProfileOverlay(sourceData, document)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "profile_overlay_incompatible", err.Error())
		return
	}
	final, err := s.renderProfileOverlayPreview(composition.ProfileYAML)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "profile_overlay_preview_failed", err.Error())
		return
	}
	effectiveInventory := inventoryFromInspection(composition.Inspection)
	writeJSON(w, http.StatusOK, ProfileOverlayPreview{
		SchemaVersion:        SchemaVersion,
		SourceID:             source.ID,
		SourceYAML:           string(sourceData),
		OverlayYAML:          string(overlayData),
		EffectiveProfileYAML: composition.ProfileYAML,
		FinalMihomoYAML:      final,
		OriginalInventory:    source.Inventory,
		EffectiveInventory:   effectiveInventory,
		Diff:                 diffInventory(source.Digest, source.Inventory, effectiveInventory),
		Validation:           "结构组合通过；实际应用前仍会运行 mihomo 引擎校验。",
	})
}

func (s *Server) renderProfileOverlayPreview(effectiveProfile string) (string, error) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return "", err
	}
	temporary, err := os.MkdirTemp(s.store.Dir(), ".profile-overlay-preview-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	profilePath := filepath.Join(temporary, "effective-profile.yaml")
	if err := os.WriteFile(profilePath, []byte(effectiveProfile), 0o600); err != nil {
		return "", err
	}
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = profilePath
	rendered, err := mihomo.RenderConfig(cfg)
	if err != nil {
		return "", err
	}
	return redactProfilePreviewSecrets(rendered)
}

func redactProfilePreviewSecrets(rendered string) (string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(rendered), &document); err != nil {
		return "", fmt.Errorf("parse rendered preview for redaction: %w", err)
	}
	if len(document.Content) == 0 {
		return rendered, nil
	}
	root := document.Content[0]
	proxies := profilePreviewMappingValue(root, "proxies")
	if proxies == nil || proxies.Kind != yaml.SequenceNode {
		return rendered, nil
	}
	redacted := false
	for _, proxy := range proxies.Content {
		if proxy.Kind != yaml.MappingNode {
			continue
		}
		name := profilePreviewMappingValue(proxy, "name")
		if name == nil || name.Value != config.TailscaleProxyName {
			continue
		}
		authKey := profilePreviewMappingValue(proxy, "auth-key")
		if authKey == nil {
			continue
		}
		authKey.Kind = yaml.ScalarNode
		authKey.Tag = "!!str"
		authKey.Value = "<redacted>"
		authKey.Style = yaml.DoubleQuotedStyle
		redacted = true
	}
	if !redacted {
		return rendered, nil
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return "", fmt.Errorf("render redacted profile preview: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("render redacted profile preview: %w", err)
	}
	return out.String(), nil
}

func profilePreviewMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func (s *Server) profileOverlayResponse() (ProfileOverlayResponse, error) {
	data, document, revision, err := s.loadProfileOverlay()
	if err != nil {
		return ProfileOverlayResponse{}, err
	}
	desired := false
	applied := false
	cfg, cfgErr := config.LoadRuntime(s.configPath)
	if cfgErr == nil {
		effectiveRevision := ""
		if document.Enabled {
			effectiveRevision = revision
		}
		desired = cfg.Mihomo.ProfileOverlayDigest == effectiveRevision
		if desired && cfg.Mihomo.ProfileMode == config.MihomoProfileModeImported {
			desiredProfile, profileErr := config.MihomoProfileDigest(cfg)
			if profileErr == nil {
				if state, exists, stateErr := runtime.LoadState(runtime.NewPaths(cfg).StateFile); stateErr == nil && exists {
					applied = state.ProfileDigest == desiredProfile
				}
			}
		}
	}
	return ProfileOverlayResponse{
		SchemaVersion: SchemaVersion,
		Revision:      revision,
		YAML:          string(data),
		Document:      document,
		Desired:       desired,
		Applied:       applied,
		Validation:    "附加配置结构有效；每个来源会单独检查名称冲突和引用。",
	}, nil
}

func (s *Server) loadProfileOverlay() ([]byte, mihomo.ProfileOverlayDocument, string, error) {
	data, err := s.store.ProfileOverlay()
	if errors.Is(err, os.ErrNotExist) {
		data, err = mihomo.RenderProfileOverlay(mihomo.DefaultProfileOverlayDocument())
	}
	if err != nil {
		return nil, mihomo.ProfileOverlayDocument{}, "", err
	}
	document, err := mihomo.ParseProfileOverlay(data)
	if err != nil {
		return data, mihomo.ProfileOverlayDocument{}, "", fmt.Errorf("stored global profile overlay is invalid: %w", err)
	}
	return data, document, mihomo.ProfileOverlayDigest(data), nil
}

func inventoryFromInspection(inspection mihomo.ImportedProfileInspection) Inventory {
	return normalizeInventory(Inventory{
		Proxies:        inspection.Proxies,
		ProxyProviders: inspection.ProxyProviders,
		ProxyGroups:    inspection.ProxyGroups,
		RuleProviders:  inspection.RuleProviders,
		RuleCount:      inspection.RuleCount,
		TerminalMatch:  inspection.TerminalMatch,
		Warnings:       inspection.Warnings,
	})
}
