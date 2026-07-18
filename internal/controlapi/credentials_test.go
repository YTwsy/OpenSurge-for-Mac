package controlapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileCredentialStorePersistsRestrictedCredentials(t *testing.T) {
	root := t.TempDir()
	credentials := NewFileCredentialStore(root)
	value := "https://example.com/profile?token=secret"
	if err := credentials.Put(t.Context(), "source-1", value); err != nil {
		t.Fatal(err)
	}
	if got, err := credentials.Get(t.Context(), "source-1"); err != nil || got != value {
		t.Fatalf("credential=%q err=%v", got, err)
	}
	directory := filepath.Join(root, "credentials")
	if info, err := os.Stat(directory); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("credential directory mode=%v err=%v", modePerm(info), err)
	}
	path := filepath.Join(directory, "sources.json")
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("credential file mode=%v err=%v", modePerm(info), err)
	}
}

func TestLegacyKeychainCredentialMigrationRunsOnce(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSources([]Source{{ID: "source-1", Origin: "https://example.com/profile"}}); err != nil {
		t.Fatal(err)
	}
	target := NewFileCredentialStore(store.Dir())
	legacy := &countingCredentialReader{value: "https://example.com/profile?token=secret"}
	if err := migrateLegacyKeychainCredentials(t.Context(), store, target, legacy); err != nil {
		t.Fatal(err)
	}
	if got, err := target.Get(t.Context(), "source-1"); err != nil || got != legacy.value {
		t.Fatalf("credential=%q err=%v", got, err)
	}
	if legacy.calls != 1 {
		t.Fatalf("legacy calls=%d", legacy.calls)
	}
	if err := migrateLegacyKeychainCredentials(t.Context(), store, target, legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.calls != 1 {
		t.Fatalf("legacy migration repeated, calls=%d", legacy.calls)
	}
	marker := filepath.Join(store.Dir(), "credentials", legacyKeychainMigrationMarker)
	if info, err := os.Stat(marker); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("migration marker mode=%v err=%v", modePerm(info), err)
	}
}

func TestLegacyKeychainFailureDoesNotBlockOrRepeat(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSources([]Source{{ID: "source-1", Origin: "https://example.com/profile"}}); err != nil {
		t.Fatal(err)
	}
	legacy := &countingCredentialReader{err: errors.New("authorization denied")}
	target := NewFileCredentialStore(store.Dir())
	if err := migrateLegacyKeychainCredentials(t.Context(), store, target, legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.calls != 1 {
		t.Fatalf("legacy calls=%d", legacy.calls)
	}
	if err := migrateLegacyKeychainCredentials(t.Context(), store, target, legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.calls != 1 {
		t.Fatalf("failed legacy migration repeated, calls=%d", legacy.calls)
	}
	if _, err := target.Get(t.Context(), "source-1"); err == nil {
		t.Fatal("missing legacy credential unexpectedly migrated")
	}
}

type countingCredentialReader struct {
	value string
	err   error
	calls int
}

func (r *countingCredentialReader) Get(context.Context, string) (string, error) {
	r.calls++
	return r.value, r.err
}

func modePerm(info os.FileInfo) os.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode().Perm()
}
