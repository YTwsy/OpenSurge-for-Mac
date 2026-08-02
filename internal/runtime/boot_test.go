package runtime

import (
	"testing"
	"time"
)

func TestStateBelongsToBootPrefersSessionID(t *testing.T) {
	bootedAt := time.Unix(1_700_000_000, 0)
	state := State{BootSessionID: "boot-a", StartedAt: bootedAt.Add(time.Hour)}
	if !state.BelongsToBoot(BootSession{ID: "BOOT-A", StartedAt: bootedAt}) {
		t.Fatal("matching boot session ID was rejected")
	}
	if state.BelongsToBoot(BootSession{ID: "boot-b", StartedAt: bootedAt.Add(-time.Hour)}) {
		t.Fatal("different boot session ID was accepted")
	}
}

func TestStateBelongsToBootMigratesLegacyStartedAt(t *testing.T) {
	bootedAt := time.Unix(1_700_000_000, 0)
	if !(State{StartedAt: bootedAt.Add(time.Second)}).BelongsToBoot(BootSession{StartedAt: bootedAt}) {
		t.Fatal("legacy state from the current boot was rejected")
	}
	if (State{StartedAt: bootedAt.Add(-time.Second)}).BelongsToBoot(BootSession{StartedAt: bootedAt}) {
		t.Fatal("legacy state from a previous boot was accepted")
	}
	if (State{}).BelongsToBoot(BootSession{StartedAt: bootedAt}) {
		t.Fatal("state without boot evidence was accepted")
	}
}

func TestParseDarwinBootTime(t *testing.T) {
	got := parseDarwinBootTime("{ sec = 1700000000, usec = 123456 } Tue Nov 14 22:13:20 2023")
	if got.Unix() != 1_700_000_000 {
		t.Fatalf("boot time = %v", got)
	}
	if got := parseDarwinBootTime("unexpected"); !got.IsZero() {
		t.Fatalf("unexpected boot time = %v", got)
	}
}
