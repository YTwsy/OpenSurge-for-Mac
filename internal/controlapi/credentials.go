package controlapi

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const sourceCredentialService = "com.opensurge.sources"

type SourceCredentialStore interface {
	Put(context.Context, string, string) error
	Get(context.Context, string) (string, error)
}

type KeychainCredentialStore struct{}

func (KeychainCredentialStore) Put(ctx context.Context, id, value string) error {
	if id == "" || value == "" {
		return fmt.Errorf("source credential id and value are required")
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "add-generic-password", "-U", "-a", id, "-s", sourceCredentialService, "-w", value)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("save source credential in Keychain: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (KeychainCredentialStore) Get(ctx context.Context, id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("source credential id is required")
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-a", id, "-s", sourceCredentialService, "-w")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read source credential from Keychain: %w", err)
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", fmt.Errorf("source credential is empty")
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
			return fmt.Errorf("migrate source %s credential to Keychain: %w", sources[index].ID, err)
		}
		sources[index].FetchURL = ""
		changed = true
	}
	if changed {
		return store.SaveSources(sources)
	}
	return nil
}
