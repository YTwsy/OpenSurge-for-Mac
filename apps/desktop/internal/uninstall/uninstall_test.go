package uninstall

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"open-mihomo-gateway/apps/desktop/internal/loginitem"
	"open-mihomo-gateway/apps/desktop/internal/menustatus"
)

type fakeLogin struct {
	state                    string
	failDisable, failRestore bool
	changes                  []bool
}

func (f *fakeLogin) Snapshot() loginitem.Snapshot { return loginitem.Snapshot{State: f.state} }
func (f *fakeLogin) SetEnabled(enabled bool) loginitem.Snapshot {
	f.changes = append(f.changes, enabled)
	if (!enabled && f.failDisable) || (enabled && f.failRestore) {
		return loginitem.Snapshot{State: f.state, Failed: true}
	}
	if enabled {
		f.state = "enabled"
	} else {
		f.state = "disabled"
	}
	return f.Snapshot()
}
func safeStatus() *menustatus.Status {
	return &menustatus.Status{SchemaVersion: 1, Gateway: "stopped", DHCP: "stopped", Mihomo: "stopped", PFAnchor: "unloaded", Forwarding: "enabled", RecoveryRequired: true, RecoveryStage: "mac_static"}
}
func TestGuardAndConfirmationRecheck(t *testing.T) {
	s := safeStatus()
	calls := 0
	login := &fakeLogin{state: "disabled"}
	m := New("preview", func() error { return nil }, func(context.Context) (*menustatus.Status, error) { return s, nil }, func(Mode) error { calls++; return nil }, login)
	if !errors.Is(m.Run(context.Background(), KeepData), ErrUnavailable) || calls != 0 {
		t.Fatal("preview can uninstall")
	}
	m.state = "available"
	if err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Mihomo = "running (version)"
	if !errors.Is(m.Run(context.Background(), KeepData), ErrUnsafe) || calls != 0 {
		t.Fatal("state was not rechecked")
	}
	s.Mihomo = "stopped"
	s.DHCP = ""
	if !errors.Is(m.Run(context.Background(), KeepData), ErrUnsafe) {
		t.Fatal("unknown services allowed")
	}
}
func TestLoginRestorationAndNoDuplicateUninstall(t *testing.T) {
	for _, outcome := range []string{"success", "cancel", "failure", "login-failure", "restore-failure"} {
		t.Run(outcome, func(t *testing.T) {
			login := &fakeLogin{state: "enabled", failDisable: outcome == "login-failure", failRestore: outcome == "restore-failure"}
			calls := 0
			m := New("available", func() error { return nil }, func(context.Context) (*menustatus.Status, error) { return safeStatus(), nil }, func(mode Mode) error {
				calls++
				if mode != KeepData {
					t.Fatal(mode)
				}
				switch outcome {
				case "success":
					return nil
				case "cancel":
					return ErrCancelled
				default:
					return ErrFailed
				}
			}, login)
			err := m.Run(context.Background(), KeepData)
			switch outcome {
			case "success":
				if err != nil || login.state != "disabled" {
					t.Fatal(err, login)
				}
				m.Run(context.Background(), KeepData)
				if calls != 1 {
					t.Fatal("duplicate uninstall")
				}
			case "login-failure":
				if !errors.Is(err, ErrLogin) || calls != 0 {
					t.Fatal(err, calls)
				}
			case "restore-failure":
				if !errors.Is(err, ErrRestore) {
					t.Fatal(err)
				}
			default:
				if err == nil || login.state != "enabled" || !reflect.DeepEqual(login.changes, []bool{false, true}) {
					t.Fatal(err, login)
				}
			}
		})
	}
}
func TestClosedModes(t *testing.T) {
	for _, mode := range []Mode{KeepData, RemoveAll} {
		exe, args, err := command(mode)
		if err != nil || exe != "/usr/bin/osascript" || len(args) != 2 || args[0] != "-e" || args[1] != `do shell script "exec '/Library/Application Support/OpenSurge/share/uninstall-gui.sh' --`+string(mode)+`" with administrator privileges` {
			t.Fatal(exe, args, err)
		}
	}
	if _, _, err := command(Mode("keep-data; echo unsafe")); err == nil {
		t.Fatal("untrusted command accepted")
	}
}

func TestUnresolvedLoginStateCannotBypassUninstallCleanup(t *testing.T) {
	for _, state := range []string{"not_found", "unavailable", "unknown"} {
		login := &fakeLogin{state: state}
		calls := 0
		m := New("available", func() error { return nil }, func(context.Context) (*menustatus.Status, error) { return safeStatus(), nil }, func(Mode) error {
			calls++
			return nil
		}, login)
		if err := m.Run(context.Background(), KeepData); !errors.Is(err, ErrLogin) || calls != 0 || len(login.changes) != 0 {
			t.Fatal(state, err, calls, login.changes)
		}
	}
}
