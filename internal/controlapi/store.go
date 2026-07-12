package controlapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Store struct {
	dir string
	mu  sync.Mutex
}

func NewStore(dir string) *Store { return &Store{dir: dir} }

func (s *Store) Dir() string { return s.dir }

func (s *Store) Ensure() error {
	for _, dir := range []string{s.dir, filepath.Join(s.dir, "sources"), filepath.Join(s.dir, "operations")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Token() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.dir, "control-token")
	data, err := os.ReadFile(path)
	if err == nil {
		return string(data), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	if err := writeAtomic(path, []byte(token), 0o600); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) Recovery() (RecoveryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var state RecoveryState
	err := readJSON(filepath.Join(s.dir, "recovery.json"), &state)
	if errors.Is(err, os.ErrNotExist) {
		return RecoveryState{SchemaVersion: SchemaVersion, Stage: RecoveryIdle}, nil
	}
	return state, err
}

func (s *Store) SaveRecovery(state RecoveryState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state.SchemaVersion = SchemaVersion
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.dir, "recovery.json"), append(data, '\n'), 0o600)
}

func (s *Store) SaveRecoveryCard(state RecoveryState) error {
	if state.NetworkSnapshot == nil {
		return fmt.Errorf("recovery card requires a network snapshot")
	}
	snapshot := state.NetworkSnapshot
	card := fmt.Sprintf(`OpenSurge for Mac - same-WiFi DHCP recovery card

Created: %s
Network service: %s
Interface: %s
Original IPv4: %s
Subnet mask: %s
Original router: %s
Original DNS: %v

Recovery order:
1. Open the router administration page at the original router address.
2. Re-enable the router DHCP server.
3. Confirm another device can obtain an automatic address from the router.
4. Restore the Mac network service to DHCP in OpenSurge, or run:
   networksetup -setdhcp %q
   networksetup -setdnsservers %q Empty
5. Reconnect clients and confirm automatic addressing and Internet access.

Do not restore the Mac to DHCP before the router DHCP server is back.
`, time.Now().UTC().Format(time.RFC3339), snapshot.NetworkService, snapshot.Interface, snapshot.IPv4, snapshot.SubnetMask, snapshot.Router, snapshot.DNS, snapshot.NetworkService, snapshot.NetworkService)
	return writeAtomic(filepath.Join(s.dir, "WIFI-DHCP-RECOVERY-CARD.txt"), []byte(card), 0o600)
}

func (s *Store) SaveOperation(op Operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.dir, "operations", op.ID+".json"), append(data, '\n'), 0o600)
}

func (s *Store) Operation(id string) (Operation, error) {
	var op Operation
	if id == "" || filepath.Base(id) != id {
		return op, fmt.Errorf("invalid operation id")
	}
	err := readJSON(filepath.Join(s.dir, "operations", id+".json"), &op)
	return op, err
}

func (s *Store) Operations(limit int) ([]Operation, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "operations"))
	if err != nil {
		return nil, err
	}
	operations := make([]Operation, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var operation Operation
		if err := readJSON(filepath.Join(s.dir, "operations", entry.Name()), &operation); err == nil {
			operations = append(operations, operation)
		}
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i].UpdatedAt.After(operations[j].UpdatedAt) })
	if limit > 0 && len(operations) > limit {
		operations = operations[:limit]
	}
	return operations, nil
}

func (s *Store) Sources() ([]Source, error) {
	var sources []Source
	err := readJSON(filepath.Join(s.dir, "sources.json"), &sources)
	if errors.Is(err, os.ErrNotExist) {
		return []Source{}, nil
	}
	return sources, err
}

func (s *Store) SaveSources(sources []Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.dir, "sources.json"), append(data, '\n'), 0o600)
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".opensurge-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
