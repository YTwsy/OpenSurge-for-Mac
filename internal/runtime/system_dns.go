package runtime

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// LoadSystemDNSBackups reads original DNS settings independently of gateway
// state. Entries are keyed by service ID so a later start cannot replace a
// pending original backup with the DNS settings OpenSurge itself installed.
func LoadSystemDNSBackups(path string) (map[string]SystemDNSSnapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]SystemDNSSnapshot), nil
	}
	if err != nil {
		return nil, err
	}
	var backups map[string]SystemDNSSnapshot
	if err := json.Unmarshal(data, &backups); err != nil {
		return nil, err
	}
	if backups == nil {
		backups = make(map[string]SystemDNSSnapshot)
	}
	return backups, nil
}

// SaveSystemDNSBackups atomically replaces the independent recovery file.
// An empty set removes it after all recorded services were restored or gone.
func SaveSystemDNSBackups(path string, backups map[string]SystemDNSSnapshot) error {
	if len(backups) != 0 {
		return saveJSON(path, backups)
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
