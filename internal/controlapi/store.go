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
	"strings"
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
	return s.saveRecoveryLocked(state)
}

func (s *Store) saveRecoveryLocked(state RecoveryState) error {
	state.SchemaVersion = SchemaVersion
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.dir, "recovery.json"), append(data, '\n'), 0o600)
}

func (s *Store) SaveRecoveryCard(state RecoveryState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.NetworkSnapshot == nil {
		return fmt.Errorf("recovery card requires a network snapshot")
	}
	snapshot := state.NetworkSnapshot
	card := fmt.Sprintf(`OpenSurge for Mac - 同一 Wi-Fi DHCP 恢复卡

创建时间：%s
网络服务：%s
接口：%s
原始 IPv4：%s
子网掩码：%s
原始路由器：%s
原始 DNS：%s

恢复顺序：
1. 在浏览器打开原始路由器地址，登录路由器管理后台。
2. 进入 LAN / 网络设置 / DHCP 服务器，重新开启路由器 DHCP 并保存；保留路由器 LAN IP 不变。
3. 确认另一台设备能够从路由器自动获得 IPv4、网关和 DNS。
4. 回到 OpenSurge 执行 OFFER 探测，再将 Mac 网络服务恢复为自动 DHCP；也可在终端运行：
   networksetup -setdhcp %q
   networksetup -setdnsservers %q Empty
5. 让客户端重新连接 Wi-Fi，确认自动获取地址并能访问互联网。

重要：在确认路由器 DHCP 已恢复并通过 OFFER 探测前，不要把 Mac 切回自动 DHCP。
`, time.Now().UTC().Format(time.RFC3339), snapshot.NetworkService, snapshot.Interface, snapshot.IPv4, snapshot.SubnetMask, snapshot.Router, strings.Join(snapshot.DNS, ", "), snapshot.NetworkService, snapshot.NetworkService)
	return writeAtomic(filepath.Join(s.dir, "WIFI-DHCP-RECOVERY-CARD.txt"), []byte(card), 0o600)
}

func (s *Store) RecoveryCard() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.ReadFile(filepath.Join(s.dir, "WIFI-DHCP-RECOVERY-CARD.txt"))
}

func (s *Store) DiscardPreparedRecovery(topology string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var current RecoveryState
	if err := readJSON(filepath.Join(s.dir, "recovery.json"), &current); err != nil {
		return err
	}
	if current.Stage != RecoveryPrepared {
		return fmt.Errorf("only prepared recovery data can be discarded")
	}
	if err := s.saveRecoveryLocked(RecoveryState{SchemaVersion: SchemaVersion, Stage: RecoveryIdle, Topology: topology}); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(s.dir, "WIFI-DHCP-RECOVERY-CARD.txt")); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = s.saveRecoveryLocked(current)
		return err
	}
	return nil
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
