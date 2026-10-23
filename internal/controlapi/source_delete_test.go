package controlapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

func sourceDeleteRequest(server *Server, id, revision string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodDelete, "http://127.0.0.1:61767/api/v1/sources/"+id, nil)
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("If-Match", `"`+revision+`"`)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func importDeleteTestSource(t *testing.T, server *Server, name, payload string) Source {
	t.Helper()
	source, err := server.importReader(name, "mihomo_profile", "file:"+name+".yaml", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestSourceDeleteRemovesHistoryCredentialsAndRecordOnly(t *testing.T) {
	server := newTestServer(t)
	server.credentials = NewFileCredentialStore(server.store.Dir())
	first := importDeleteTestSource(t, server, "old", "rules: [MATCH,DIRECT]\n")
	source := importDeleteTestSource(t, server, "old", "rules: [MATCH,REJECT]\n")
	other := importDeleteTestSource(t, server, "keep", "rules: [MATCH,DIRECT]\n")
	for _, id := range []string{source.ID, other.ID} {
		if err := server.credentials.Put(t.Context(), id, "https://example.com/"+id+"?token=secret"); err != nil {
			t.Fatal(err)
		}
	}
	exported, err := server.exportSourceSnapshot(source, source.SnapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(t.TempDir(), "original.yaml")
	if err := os.WriteFile(original, []byte("rules: [MATCH,DIRECT]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A persisted path must never authorize removal outside the source ID's dir.
	sources, _ := server.store.Sources()
	sources[0].SnapshotPath = original
	sources[0].Versions[0].SnapshotPath = original
	if err := server.store.SaveSources(sources); err != nil {
		t.Fatal(err)
	}
	revision := fileDigest(server.configPath)
	response := sourceDeleteRequest(server, source.ID, revision)
	if response.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
	}
	for _, path := range []string{source.SnapshotPath, first.SnapshotPath, filepath.Dir(source.SnapshotPath)} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("deleted path still exists: %s, err=%v", path, err)
		}
	}
	for _, path := range []string{other.SnapshotPath, original, exported} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unrelated file was removed: %s, err=%v", path, err)
		}
	}
	remaining, err := NewStore(server.store.Dir()).Sources()
	if err != nil || len(remaining) != 1 || remaining[0].ID != other.ID {
		t.Fatalf("persisted sources=%#v err=%v", remaining, err)
	}
	credentials := NewFileCredentialStore(server.store.Dir())
	if _, err := credentials.Get(t.Context(), source.ID); err == nil {
		t.Fatal("deleted source credential is still readable")
	}
	if _, err := credentials.Get(t.Context(), other.ID); err != nil {
		t.Fatalf("other source credential was removed: %v", err)
	}
	if fileDigest(server.configPath) != revision {
		t.Fatal("source deletion modified gateway configuration")
	}
	if response := sourceDeleteRequest(server, source.ID, revision); response.Code != http.StatusNotFound {
		t.Fatalf("missing source status=%d", response.Code)
	}
}

func TestSourceDeleteWorksWithMissingOrInvalidSnapshots(t *testing.T) {
	for _, condition := range []string{"missing-file", "missing-directory", "invalid-yaml"} {
		t.Run(condition, func(t *testing.T) {
			server := newTestServer(t)
			payload := "rules: [MATCH,DIRECT]\n"
			if condition == "invalid-yaml" {
				payload = "rules: ["
			}
			source := importDeleteTestSource(t, server, "unused", payload)
			if condition == "missing-file" {
				if err := os.Remove(source.SnapshotPath); err != nil {
					t.Fatal(err)
				}
			} else if condition == "missing-directory" {
				if err := os.RemoveAll(filepath.Dir(source.SnapshotPath)); err != nil {
					t.Fatal(err)
				}
			}
			response := sourceDeleteRequest(server, source.ID, fileDigest(server.configPath))
			if response.Code != http.StatusOK {
				t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
			}
			list := performAuthorized(server, http.MethodGet, "/api/v1/sources", nil)
			if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"sources":[]`) {
				t.Fatalf("sources=%s", list.Body.String())
			}
		})
	}
}

func TestSourceDeleteProtectsSelectedAndHistoricalVersions(t *testing.T) {
	for _, condition := range []string{"current", "historical", "missing-selected", "composed"} {
		t.Run(condition, func(t *testing.T) {
			server := newTestServer(t)
			source := importDeleteTestSource(t, server, "selected", "rules: [MATCH,DIRECT]\n")
			cfg, err := config.LoadRuntime(server.configPath)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Mihomo.ProfileMode, cfg.Mihomo.Profile = config.MihomoProfileModeImported, source.SnapshotPath
			if condition == "composed" {
				cfg.Mihomo.ProfileSourceDigest = source.Digest
				cfg.Mihomo.Profile = filepath.Join(t.TempDir(), "composed.yaml")
				if err := os.WriteFile(cfg.Mihomo.Profile, []byte("rules: [MATCH,DIRECT]\n# composed\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(server.configPath, []byte(config.Render(cfg)), 0o600); err != nil {
				t.Fatal(err)
			}
			if condition == "historical" {
				source = importDeleteTestSource(t, server, "selected", "rules: [MATCH,REJECT]\n")
			}
			if condition == "missing-selected" {
				if err := os.Remove(source.SnapshotPath); err != nil {
					t.Fatal(err)
				}
			}
			response := sourceDeleteRequest(server, source.ID, fileDigest(server.configPath))
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "source_in_use") {
				t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
			}
			if _, err := server.sourceByID(source.ID); err != nil {
				t.Fatal("selected source was removed")
			}
		})
	}
}

func TestSourceDeleteRejectsStaleRevisionAndUncertainRuntime(t *testing.T) {
	server := newTestServer(t)
	source := importDeleteTestSource(t, server, "unused", "rules: [MATCH,DIRECT]\n")
	for _, revision := range []string{"", "stale-revision"} {
		response := sourceDeleteRequest(server, source.ID, revision)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "revision_conflict") {
			t.Fatalf("revision status=%d body=%s", response.Code, response.Body.String())
		}
	}
	cfg, err := config.LoadRuntime(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfg.Runtime.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveState(runtime.NewPaths(cfg).StateFile, runtime.State{ProfileDigest: source.Digest}); err != nil {
		t.Fatal(err)
	}
	response := sourceDeleteRequest(server, source.ID, fileDigest(server.configPath))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "source_in_use") {
		t.Fatalf("runtime conflict status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(source.SnapshotPath); err != nil {
		t.Fatal("rejected deletion removed a snapshot")
	}
}

func TestSourceDeleteDoesNotFollowSymlinks(t *testing.T) {
	for _, parent := range []bool{false, true} {
		t.Run(map[bool]string{false: "source-directory", true: "sources-parent"}[parent], func(t *testing.T) {
			server := newTestServer(t)
			source := importDeleteTestSource(t, server, "unused", "rules: [MATCH,DIRECT]\n")
			path := filepath.Dir(source.SnapshotPath)
			if parent {
				path = filepath.Dir(path)
			}
			outside := filepath.Join(t.TempDir(), "preserved")
			if err := os.Rename(path, outside); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, path); err != nil {
				t.Fatal(err)
			}
			response := sourceDeleteRequest(server, source.ID, fileDigest(server.configPath))
			wantStatus := http.StatusOK
			if parent {
				wantStatus = http.StatusInternalServerError
			}
			if response.Code != wantStatus {
				t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
			}
			preserved := filepath.Join(outside, filepath.Base(source.SnapshotPath))
			if parent {
				preserved = filepath.Join(outside, source.ID, filepath.Base(source.SnapshotPath))
			}
			if _, err := os.Stat(preserved); err != nil {
				t.Fatalf("symlink target was removed: %v", err)
			}
		})
	}
}

type failedDeleteCredentials struct{ SourceCredentialStore }

func (failedDeleteCredentials) Delete(context.Context, string) error {
	return errors.New("credential storage unavailable")
}

func TestSourceDeleteCanRetryCleanupFailure(t *testing.T) {
	server := newTestServer(t)
	source := importDeleteTestSource(t, server, "unused", "rules: [MATCH,DIRECT]\n")
	credentials := server.credentials
	server.credentials = failedDeleteCredentials{credentials}
	response := sourceDeleteRequest(server, source.ID, fileDigest(server.configPath))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("failed cleanup status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := server.sourceByID(source.ID); err != nil {
		t.Fatal("failed cleanup hid the source")
	}
	server.credentials = credentials
	response = sourceDeleteRequest(server, source.ID, fileDigest(server.configPath))
	if response.Code != http.StatusOK {
		t.Fatalf("retry status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSourceMutationsRejectConcurrentRequests(t *testing.T) {
	server := newTestServer(t)
	source := importDeleteTestSource(t, server, "unused", "rules: [MATCH,DIRECT]\n")
	server.sourcesMu.Lock()
	for _, path := range []string{"/api/v1/sources", "/api/v1/sources/" + source.ID + "/refresh", "/api/v1/sources/" + source.ID + "/apply"} {
		response := performAuthorized(server, http.MethodPost, path, nil)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "source_operation_in_progress") {
			t.Errorf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	response := sourceDeleteRequest(server, source.ID, fileDigest(server.configPath))
	server.sourcesMu.Unlock()
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "source_operation_in_progress") {
		t.Fatalf("concurrent delete status=%d body=%s", response.Code, response.Body.String())
	}
	server.lifecycleMu.Lock()
	response = sourceDeleteRequest(server, source.ID, fileDigest(server.configPath))
	server.lifecycleMu.Unlock()
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "operation_in_progress") {
		t.Fatalf("lifecycle conflict status=%d body=%s", response.Code, response.Body.String())
	}
}
