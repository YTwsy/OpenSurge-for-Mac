package controlapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const finderRevealTimeout = 5 * time.Second

func (s *Server) handleSourceSnapshotLocation(w http.ResponseWriter, r *http.Request) {
	source, path, ok := s.sourceSnapshotForAction(w, r.PathValue("id"))
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, sourceSnapshotFile(source, path, s.store.Dir(), "managed_snapshot"))
}

func (s *Server) handleSourceReveal(w http.ResponseWriter, r *http.Request) {
	source, path, ok := s.sourceSnapshotForAction(w, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.revealInFinder(r.Context(), path); err != nil {
		writeError(w, http.StatusInternalServerError, "finder_reveal_failed", "Finder could not select the managed snapshot: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sourceSnapshotFile(source, path, s.store.Dir(), "managed_snapshot"))
}

func (s *Server) handleSourceExport(w http.ResponseWriter, r *http.Request) {
	source, path, ok := s.sourceSnapshotForAction(w, r.PathValue("id"))
	if !ok {
		return
	}
	exported, err := s.exportSourceSnapshot(source, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "source_export_failed", err.Error())
		return
	}
	if err := s.revealInFinder(r.Context(), exported); err != nil {
		display := displayLocalPath(exported, s.store.Dir())
		writeError(w, http.StatusInternalServerError, "finder_reveal_failed", fmt.Sprintf("editable copy was exported to %s, but Finder could not select it: %v", display, err))
		return
	}
	writeJSON(w, http.StatusCreated, sourceSnapshotFile(source, exported, s.store.Dir(), "editable_export"))
}

func (s *Server) sourceSnapshotForAction(w http.ResponseWriter, id string) (Source, string, bool) {
	source, err := s.sourceByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "source_not_found", err.Error())
		return Source{}, "", false
	}
	path, err := validatedSourceSnapshotPath(s.store.Dir(), source)
	if err != nil {
		writeError(w, http.StatusConflict, "source_snapshot_unavailable", err.Error())
		return Source{}, "", false
	}
	return source, path, true
}

func sourceSnapshotFile(source Source, path, storeDir, kind string) SourceSnapshotFile {
	return SourceSnapshotFile{
		SchemaVersion: SchemaVersion,
		SourceID:      source.ID,
		Kind:          kind,
		Path:          path,
		DisplayPath:   displayLocalPath(path, storeDir),
	}
}

func managedSourceSnapshotPath(storeDir string, source Source) (string, error) {
	if !validHexValue(source.ID, 16) {
		return "", fmt.Errorf("source id is invalid")
	}
	if !validHexValue(source.Digest, sha256.Size*2) {
		return "", fmt.Errorf("source digest is invalid")
	}
	root, err := filepath.Abs(filepath.Join(storeDir, "sources"))
	if err != nil {
		return "", err
	}
	expected, err := filepath.Abs(filepath.Join(root, source.ID, source.Digest+".yaml"))
	if err != nil {
		return "", err
	}
	actual, err := filepath.Abs(source.SnapshotPath)
	if err != nil {
		return "", err
	}
	if actual != expected {
		return "", fmt.Errorf("source snapshot path does not match its managed id and digest")
	}
	relative, err := filepath.Rel(root, actual)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("source snapshot path is outside the managed source store")
	}
	return actual, nil
}

func validatedSourceSnapshotPath(storeDir string, source Source) (string, error) {
	path, err := managedSourceSnapshotPath(storeDir, source)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("read managed source snapshot: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("managed source snapshot is not a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("managed source snapshot must not grant group or other access")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read managed source snapshot: %w", err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != source.Digest {
		return "", fmt.Errorf("managed source snapshot content no longer matches its recorded digest; refresh or re-import the source")
	}
	return path, nil
}

func validHexValue(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func displayLocalPath(path, storeDir string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if relative, err := filepath.Rel(home, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return filepath.Join("~", relative)
		}
	}
	if relative, err := filepath.Rel(storeDir, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return filepath.Join("OpenSurge", relative)
	}
	return filepath.Base(path)
}

func (s *Server) exportSourceSnapshot(source Source, snapshotPath string) (string, error) {
	input, err := os.Open(snapshotPath)
	if err != nil {
		return "", fmt.Errorf("open managed source snapshot: %w", err)
	}
	defer input.Close()

	exportDir := filepath.Join(s.store.Dir(), "exports")
	if err := os.MkdirAll(exportDir, 0o700); err != nil {
		return "", fmt.Errorf("create source export directory: %w", err)
	}
	if err := os.Chmod(exportDir, 0o700); err != nil {
		return "", fmt.Errorf("secure source export directory: %w", err)
	}

	stem := sourceExportStem(source.Name)
	digest := source.Digest
	if len(digest) > 8 {
		digest = digest[:8]
	}
	base := fmt.Sprintf("%s-%s-%s", stem, digest, time.Now().UTC().Format("20060102-150405"))
	for attempt := 0; attempt < 100; attempt++ {
		name := base + ".yaml"
		if attempt > 0 {
			name = fmt.Sprintf("%s-%d.yaml", base, attempt+1)
		}
		path := filepath.Join(exportDir, name)
		output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("create editable source copy: %w", err)
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(path)
			if copyErr != nil {
				return "", fmt.Errorf("write editable source copy: %w", copyErr)
			}
			return "", fmt.Errorf("close editable source copy: %w", closeErr)
		}
		return path, nil
	}
	return "", fmt.Errorf("could not allocate a unique editable source filename")
}

func sourceExportStem(name string) string {
	name = strings.TrimSpace(name)
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".yaml") {
		name = name[:len(name)-len(".yaml")]
	} else if strings.HasSuffix(lower, ".yml") {
		name = name[:len(name)-len(".yml")]
	}
	var out strings.Builder
	separator := false
	count := 0
	for _, char := range name {
		if count >= 64 {
			break
		}
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '-' || char == '_' || char == '.' {
			out.WriteRune(char)
			separator = false
			count++
			continue
		}
		if !separator && out.Len() > 0 {
			out.WriteByte('-')
			separator = true
			count++
		}
	}
	stem := strings.Trim(out.String(), ".-_")
	if stem == "" {
		return "mihomo-profile"
	}
	return stem
}

func revealPathInFinder(ctx context.Context, path string) error {
	commandCtx, cancel := context.WithTimeout(ctx, finderRevealTimeout)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "/usr/bin/open", "-R", path).CombinedOutput()
	if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("open -R timed out after %s", finderRevealTimeout)
	}
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return fmt.Errorf("open -R failed: %w: %s", err, detail)
		}
		return fmt.Errorf("open -R failed: %w", err)
	}
	return nil
}
