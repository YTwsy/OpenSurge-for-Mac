package runtime

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
)

func TestSystemDNSBackupsSurviveGatewayStateRemoval(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	paths := NewPaths(cfg)
	backups, err := LoadSystemDNSBackups(paths.SystemDNSBackupFile)
	if err != nil || len(backups) != 0 {
		t.Fatalf("missing backup: %v, %v", backups, err)
	}
	backups["service-a"] = SystemDNSSnapshot{
		ServiceID: "service-a", Interface: "en0", NetworkService: "Ethernet",
		Servers: nil, Resolvers: []string{"192.0.2.1"}, Domains: []string{"home.arpa"}, Owned: true,
	}
	backups["service-b"] = SystemDNSSnapshot{
		ServiceID: "service-b", Interface: "en1", NetworkService: "Wi-Fi",
		Servers: []string{"192.0.2.2", "2001:db8::53"}, Owned: true,
		VerifiedAt: time.Unix(1_700_000_000, 0).UTC(),
	}
	if err := SaveSystemDNSBackups(paths.SystemDNSBackupFile, backups); err != nil {
		t.Fatal(err)
	}
	if err := SaveState(paths.StateFile, State{LocalSystemDNS: ptrDNSBackup(backups["service-a"])}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveState(paths.StateFile); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSystemDNSBackups(paths.SystemDNSBackupFile)
	if err != nil || !reflect.DeepEqual(got, backups) {
		t.Fatalf("backup lost or changed after gateway cleanup: %v, %v", got, err)
	}
	info, err := os.Stat(paths.SystemDNSBackupFile)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("backup permissions: %v, %v", info, err)
	}
}

func TestSystemDNSBackupsReplaceAtomicallyAndRemoveRecoveredEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "system-dns-backups.json")
	backups := map[string]SystemDNSSnapshot{
		"service-a": {ServiceID: "service-a", Servers: []string{"192.0.2.1"}},
		"service-b": {ServiceID: "service-b"},
	}
	if err := SaveSystemDNSBackups(path, backups); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	oldFile, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer oldFile.Close()
	delete(backups, "service-a")
	if err := SaveSystemDNSBackups(path, backups); err != nil {
		t.Fatal(err)
	}
	// An already-open reader must retain the complete previous file, rather
	// than seeing the recovery record truncated and rewritten in place.
	oldData, err := io.ReadAll(oldFile)
	if err != nil || string(oldData) != string(original) {
		t.Fatalf("existing reader observed an in-place rewrite: %q, %v", oldData, err)
	}
	got, err := LoadSystemDNSBackups(path)
	if err != nil || !reflect.DeepEqual(got, backups) {
		t.Fatalf("remaining backups: %v, %v", got, err)
	}
	if err := SaveSystemDNSBackups(path, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty backup file was not removed: %v", err)
	}
	if err := SaveSystemDNSBackups(path, map[string]SystemDNSSnapshot{}); err != nil {
		t.Fatalf("empty backup removal is not idempotent: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 0 {
		t.Fatalf("left recovery temporary files: %v, %v", entries, err)
	}
}

func TestSystemDNSBackupsDistinguishUnreadableDataFromNoBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "system-dns-backups.json")
	if err := os.WriteFile(path, []byte("{\"service-a\":"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSystemDNSBackups(path); err == nil {
		t.Fatal("malformed recovery data was treated as no backup")
	}
	if _, err := LoadSystemDNSBackups(filepath.Dir(path)); err == nil {
		t.Fatal("unreadable recovery file was treated as no backup")
	}
	if err := os.WriteFile(path, []byte("null\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	backups, err := LoadSystemDNSBackups(path)
	if err != nil || backups == nil || len(backups) != 0 {
		t.Fatalf("empty recovery data: %v, %v", backups, err)
	}
}

func ptrDNSBackup(snapshot SystemDNSSnapshot) *SystemDNSSnapshot { return &snapshot }
