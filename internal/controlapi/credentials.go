package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const (
	legacySourceCredentialService = "com.opensurge.sources"
	sourceCredentialSchemaVersion = 1
	legacyKeychainMigrationMarker = ".legacy-keychain-migration-v1"
)

type SourceCredentialStore interface {
	Put(context.Context, string, string) error
	Get(context.Context, string) (string, error)
}

type sourceCredentialReader interface {
	Get(context.Context, string) (string, error)
}

type fileCredentialDocument struct {
	SchemaVersion int               `json:"schema_version"`
	Sources       map[string]string `json:"sources"`
}

type FileCredentialStore struct {
	path string
	mu   sync.Mutex
}

func NewFileCredentialStore(storeDir string) *FileCredentialStore {
	return &FileCredentialStore{path: filepath.Join(storeDir, "credentials", "sources.json")}
}

func (f *FileCredentialStore) Put(ctx context.Context, id, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id == "" || value == "" {
		return fmt.Errorf("source credential id and value are required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	document, err := f.load()
	if err != nil {
		return err
	}
	document.Sources[id] = value
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(f.path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("save source credential file: %w", err)
	}
	return nil
}

func (f *FileCredentialStore) Get(ctx context.Context, id string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if id == "" {
		return "", fmt.Errorf("source credential id is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	document, err := f.load()
	if err != nil {
		return "", err
	}
	value := document.Sources[id]
	if value == "" {
		return "", fmt.Errorf("source credential not found")
	}
	return value, nil
}

func (f *FileCredentialStore) load() (fileCredentialDocument, error) {
	document := fileCredentialDocument{SchemaVersion: sourceCredentialSchemaVersion, Sources: map[string]string{}}
	data, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return document, nil
	}
	if err != nil {
		return fileCredentialDocument{}, fmt.Errorf("read source credential file: %w", err)
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return fileCredentialDocument{}, fmt.Errorf("decode source credential file: %w", err)
	}
	if document.SchemaVersion != sourceCredentialSchemaVersion {
		return fileCredentialDocument{}, fmt.Errorf("unsupported source credential schema version %d", document.SchemaVersion)
	}
	if document.Sources == nil {
		document.Sources = map[string]string{}
	}
	return document, nil
}

// LegacyKeychainCredentialReader is used only by the one-time upgrade migration.
// New imports and refreshes use FileCredentialStore and never write to Keychain.
type LegacyKeychainCredentialReader struct{}

func (LegacyKeychainCredentialReader) Get(ctx context.Context, id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("source credential id is required")
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-a", id, "-s", legacySourceCredentialService, "-w")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read legacy source credential from Keychain: %w", err)
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", fmt.Errorf("legacy source credential is empty")
	}
	return value, nil
}

type memoryCredentialStore struct{ values map[string]string }

func (m *memoryCredentialStore) Put(_ context.Context, id, value string) error {
	if m.values == nil {
		m.values = map[string]string{}
	}
	m.values[id] = value
	return nil
}

func (m *memoryCredentialStore) Get(_ context.Context, id string) (string, error) {
	value := m.values[id]
	if value == "" {
		return "", fmt.Errorf("credential not found")
	}
	return value, nil
}

func migrateSourceCredentials(ctx context.Context, store *Store, credentials SourceCredentialStore) error {
	sources, err := store.Sources()
	if err != nil {
		return err
	}
	changed := false
	for index := range sources {
		if sources[index].FetchURL == "" {
			continue
		}
		if err := credentials.Put(ctx, sources[index].ID, sources[index].FetchURL); err != nil {
			return fmt.Errorf("migrate source %s credential to local store: %w", sources[index].ID, err)
		}
		sources[index].FetchURL = ""
		changed = true
	}
	if changed {
		return store.SaveSources(sources)
	}
	return nil
}

func migrateLegacyKeychainCredentials(ctx context.Context, store *Store, target SourceCredentialStore, legacy sourceCredentialReader) error {
	marker := filepath.Join(store.Dir(), "credentials", legacyKeychainMigrationMarker)
	if _, err := os.Stat(marker); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect legacy Keychain migration marker: %w", err)
	}
	sources, err := store.Sources()
	if err != nil {
		return err
	}
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !strings.HasPrefix(source.Origin, "https://") {
			continue
		}
		if value, err := target.Get(ctx, source.ID); err == nil && value != "" {
			continue
		}
		value, err := legacy.Get(ctx, source.ID)
		if err != nil {
			// A locked, denied, or absent legacy item must not prevent the Control
			// Service from starting or cause repeated authorization attempts.
			continue
		}
		if err := target.Put(ctx, source.ID, value); err != nil {
			return fmt.Errorf("migrate source %s credential from legacy Keychain: %w", source.ID, err)
		}
	}
	return writeAtomic(marker, []byte("migration attempted\n"), 0o600)
}
