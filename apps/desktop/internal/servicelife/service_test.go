package servicelife

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"open-mihomo-gateway/apps/desktop/internal/menustatus"
)

func stopped() *menustatus.Status {
	return &menustatus.Status{SchemaVersion: 1, Gateway: "stopped", DHCP: "stopped", Mihomo: "stopped", PFAnchor: "unloaded", Forwarding: "enabled"}
}
func TestWakeUsesOnlyExistingUserAgentAndNeverRestarts(t *testing.T) {
	var calls [][]string
	c := New(501, "/Users/test", true, func(ctx context.Context, executable string, args ...string) error {
		calls = append(calls, append([]string{executable}, args...))
		if args[0] == "print" {
			return errors.New("not loaded")
		}
		return nil
	}, func(context.Context) (*menustatus.Status, error) { return stopped(), nil })
	if err := c.Wake(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"/bin/launchctl", "print", "gui/501/com.opensurge.control"}, {"/bin/launchctl", "bootstrap", "gui/501", "/Users/test/Library/LaunchAgents/com.opensurge.control.plist"}, {"/bin/launchctl", "kickstart", "gui/501/com.opensurge.control"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected commands: %v", calls)
	}
}
func TestFullQuitRejectsUnavailableActiveRecoveryAndIncompleteEvidence(t *testing.T) {
	for _, scenario := range []string{"unavailable", "running", "recovery", "dhcp", "mihomo", "pf", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			s := stopped()
			var readError error
			switch scenario {
			case "unavailable":
				readError = errors.New("offline")
			case "running":
				s.Gateway = "running"
			case "recovery":
				s.RecoveryRequired = true
				s.RecoveryStage = "mac_static"
			case "dhcp":
				s.DHCP = "running"
			case "mihomo":
				s.Mihomo = "running"
			case "pf":
				s.PFAnchor = "loaded"
			case "missing":
				s.DHCP = ""
			}
			c := New(501, "/Users/test", true, func(context.Context, string, ...string) error { t.Fatal("unsafe state reached launchctl"); return nil }, func(context.Context) (*menustatus.Status, error) { return s, readError })
			if !errors.Is(c.Stop(context.Background()), ErrUnsafe) {
				t.Fatal("unsafe quit allowed")
			}
		})
	}
}
func TestQuitSerializesWithWakeAndBlocksAllLaterWakes(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var calls []string
	c := New(501, "/Users/test", true, func(ctx context.Context, exe string, args ...string) error {
		mu.Lock()
		calls = append(calls, args[0])
		mu.Unlock()
		if args[0] == "bootout" {
			close(started)
			<-release
		}
		return nil
	}, func(context.Context) (*menustatus.Status, error) { return stopped(), nil })
	stopResult, wakeResult := make(chan error, 1), make(chan error, 1)
	go func() { stopResult <- c.Stop(context.Background()) }()
	<-started
	go func() { wakeResult <- c.Wake(context.Background()) }()
	close(release)
	if err := <-stopResult; err != nil {
		t.Fatal(err)
	}
	if !errors.Is(<-wakeResult, ErrQuitting) {
		t.Fatal("wake resurrected service during quit")
	}
	if !reflect.DeepEqual(calls, []string{"bootout"}) {
		t.Fatalf("unexpected quit commands %v", calls)
	}
}
func TestFailureReopensWakeGateAndFixtureCannotManageInstalledJob(t *testing.T) {
	fail := true
	c := New(501, "/Users/test", true, func(ctx context.Context, exe string, args ...string) error {
		if fail {
			return errors.New("denied")
		}
		return nil
	}, func(context.Context) (*menustatus.Status, error) { return stopped(), nil })
	if !errors.Is(c.Stop(context.Background()), ErrCommand) {
		t.Fatal("stop failure hidden")
	}
	fail = false
	if err := c.Wake(context.Background()); err != nil {
		t.Fatal("failed quit left host unusable", err)
	}
	fixture := New(501, "/Users/test", false, func(context.Context, string, ...string) error { t.Fatal("fixture touched launchd"); return nil }, nil)
	if !errors.Is(fixture.Wake(context.Background()), ErrUnavailable) || !errors.Is(fixture.Stop(context.Background()), ErrUnavailable) {
		t.Fatal("fixture was not isolated")
	}
}
