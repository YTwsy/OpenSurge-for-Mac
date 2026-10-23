package controlapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

// Serialize the complete read/modify/write transaction, including refresh's
// credential update, so a concurrent import or apply cannot resurrect a row.
func (s *Server) sourceMutation(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.sourcesMu.TryLock() {
			writeError(w, http.StatusConflict, "source_operation_in_progress", "another source operation is already running; retry when it finishes")
			return
		}
		defer s.sourcesMu.Unlock()
		next(w, r)
	})
}

func (s *Server) handleSourceDelete(w http.ResponseWriter, r *http.Request) {
	if !s.lifecycleMu.TryLock() {
		writeError(w, http.StatusConflict, "operation_in_progress", "another gateway lifecycle operation is already running")
		return
	}
	defer s.lifecycleMu.Unlock()
	match := strings.Trim(r.Header.Get("If-Match"), `"`)
	if match == "" || match != fileDigest(s.configPath) {
		writeError(w, http.StatusConflict, "revision_conflict", "If-Match must contain the current config revision")
		return
	}
	sources, err := s.store.Sources()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sources_failed", err.Error())
		return
	}
	index := -1
	for i := range sources {
		if sources[i].ID == r.PathValue("id") {
			index = i
			break
		}
	}
	if index < 0 {
		writeError(w, http.StatusNotFound, "source_not_found", "source not found")
		return
	}
	source := sources[index]
	if err := s.checkSourceUnused(source); err != nil {
		writeError(w, http.StatusConflict, "source_in_use", err.Error())
		return
	}
	if !validHexValue(source.ID, 16) {
		writeError(w, http.StatusConflict, "source_delete_failed", "source id is invalid")
		return
	}
	// Keep the row until all cleanup succeeds. A missing snapshot or credential
	// is harmless, and a failed cleanup can be retried through the same button.
	if err := s.removeSourceSnapshots(source.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "source_delete_failed", err.Error())
		return
	}
	if err := s.credentials.Delete(r.Context(), source.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "source_delete_failed", err.Error())
		return
	}
	remaining := append(sources[:index:index], sources[index+1:]...)
	if err := s.store.SaveSources(remaining); err != nil {
		writeError(w, http.StatusInternalServerError, "source_delete_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": SchemaVersion, "deleted": source.ID})
}

func (s *Server) checkSourceUnused(source Source) error {
	cfg, err := config.LoadRuntime(s.configPath)
	if err != nil {
		return fmt.Errorf("cannot verify the selected source: %w", err)
	}
	versions := append([]SourceVersion{sourceVersion(source)}, source.Versions...)
	for _, version := range versions {
		if cfg.Mihomo.ProfileMode == config.MihomoProfileModeImported && filepath.Clean(cfg.Mihomo.Profile) == filepath.Clean(version.SnapshotPath) {
			return fmt.Errorf("this source is selected; apply another source before deleting it")
		}
	}
	desiredProfile, err := config.MihomoProfileDigest(cfg)
	if err != nil {
		return fmt.Errorf("cannot verify the selected profile: %w", err)
	}
	desiredSource := cfg.Mihomo.ProfileSourceDigest
	if desiredSource == "" {
		desiredSource = desiredProfile
	}
	for _, version := range versions {
		if desiredSource != "" && version.Digest == desiredSource {
			return fmt.Errorf("this source or a historical version is selected; apply another source before deleting it")
		}
	}
	state, exists, err := runtime.LoadState(runtime.NewPaths(cfg).StateFile)
	if err != nil {
		return fmt.Errorf("cannot verify the running profile: %w", err)
	}
	// When desired and applied diverge, the runtime digest alone does not tell
	// us which raw source and overlay produced it. Require reconciliation first.
	if exists && state.ProfileDigest != "" && state.ProfileDigest != desiredProfile {
		return fmt.Errorf("the running profile differs from the selected profile; apply the selected profile or stop the gateway before deleting sources")
	}
	return nil
}

func (s *Server) removeSourceSnapshots(id string) error {
	root, err := os.OpenRoot(s.store.Dir())
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat("sources")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed source directory is not a regular directory")
	}
	// Never trust persisted SnapshotPath values for deletion. Root also confines
	// traversal if a directory is replaced by a symlink during cleanup.
	return root.RemoveAll(filepath.Join("sources", id))
}
