package runtime

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := State{
		PIDDNSMasq:         101,
		PIDMihomo:          202,
		IPForwardingBefore: "0",
		PFEnabledBefore:    true,
		PFAnchorLoaded:     true,
		StartedAt:          time.Unix(1_700_000_000, 0).UTC(),
	}

	if err := SaveState(path, want); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	got, exists, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if !exists {
		t.Fatalf("LoadState() exists = false")
	}
	if got != want {
		t.Fatalf("LoadState() = %+v, want %+v", got, want)
	}
}

func TestSaveStateReplacesExistingState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first := State{PIDDNSMasq: 101, IPForwardingBefore: "0"}
	second := State{PIDMihomo: 202, IPForwardingBefore: "1", PFAnchorLoaded: true}

	if err := SaveState(path, first); err != nil {
		t.Fatalf("SaveState(first) error = %v", err)
	}
	if err := SaveState(path, second); err != nil {
		t.Fatalf("SaveState(second) error = %v", err)
	}
	got, exists, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if !exists {
		t.Fatalf("LoadState() exists = false")
	}
	if got != second {
		t.Fatalf("LoadState() = %+v, want %+v", got, second)
	}
}

func TestSaveStateFailureLeavesExistingState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	existing := State{PIDDNSMasq: 101, IPForwardingBefore: "0"}

	if err := SaveState(path, existing); err != nil {
		t.Fatalf("SaveState(existing) error = %v", err)
	}
	err := SaveState(filepath.Join(dir, "missing", "state.json"), State{PIDMihomo: 202})
	if err == nil {
		t.Fatalf("SaveState() succeeded with missing parent directory")
	}
	got, exists, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState(existing) error = %v", err)
	}
	if !exists {
		t.Fatalf("LoadState(existing) exists = false")
	}
	if got != existing {
		t.Fatalf("LoadState(existing) = %+v, want %+v", got, existing)
	}
}
