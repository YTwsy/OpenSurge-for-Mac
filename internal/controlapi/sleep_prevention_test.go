package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSleepPreventionControllerDefaultsOffAndDoesNotPersistIntent(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeSleepPreventionRunner{}
	controller := newSleepPreventionController(runner, filepath.Join(dir, "config.yaml"))
	if got := controller.Status(); got.Enabled || got.Active || got.Error != "" {
		t.Fatalf("default status=%#v", got)
	}

	status, err := controller.SetEnabled(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || !status.Active || runner.Count() != 1 {
		t.Fatalf("enabled status=%#v acquisitions=%d", status, runner.Count())
	}
	status, err = controller.SetEnabled(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.Active {
		t.Fatalf("disabled status=%#v", status)
	}
	if _, err := os.Stat(filepath.Join(dir, "preferences.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-persistent switch wrote preferences: %v", err)
	}
}

func TestSleepPreventionControllerReportsUnexpectedLeaseLoss(t *testing.T) {
	runner := &fakeSleepPreventionRunner{}
	controller := newSleepPreventionController(runner, "/tmp/config.yaml")
	if _, err := controller.SetEnabled(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	runner.LastLease().Fail(errors.New("helper restarted"))
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status := controller.Status()
		if !status.Active && status.Error != "" {
			if status.Enabled {
				t.Fatalf("lost lease remained enabled: %#v", status)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("lease loss status=%#v", controller.Status())
}

func TestParseSystemSleepDisabled(t *testing.T) {
	for _, test := range []struct {
		name   string
		output string
		want   bool
	}{
		{name: "enabled", output: "System-wide power settings:\n SleepDisabled 1\n", want: true},
		{name: "disabled", output: "System-wide power settings:\n SleepDisabled 0\n", want: false},
		{name: "unset", output: "System-wide power settings:\nCurrently in use:\n sleep 10\n", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := parseSystemSleepDisabled(test.output); got != test.want {
				t.Fatalf("parseSystemSleepDisabled()=%t, want %t", got, test.want)
			}
		})
	}
}

func TestSystemSleepLeaseManagerUsesReferenceCountAndOwnershipMarker(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "sleep-prevention-owned")
	manager := &systemSleepLeaseManager{marker: marker}
	manager.inspect = func(context.Context) (bool, error) { return false, nil }
	var changes []bool
	manager.set = func(_ context.Context, disabled bool) error {
		changes = append(changes, disabled)
		return nil
	}
	if err := manager.Acquire(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Acquire(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("ownership marker missing: %v", err)
	}
	if err := manager.Release(); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || !changes[0] {
		t.Fatalf("changes after first release=%v", changes)
	}
	if err := manager.Release(); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[1] {
		t.Fatalf("changes after final release=%v", changes)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ownership marker remained after release: %v", err)
	}
}

func TestSystemSleepLeaseManagerRejectsExistingExternalDisable(t *testing.T) {
	manager := &systemSleepLeaseManager{marker: filepath.Join(t.TempDir(), "sleep-prevention-owned")}
	manager.inspect = func(context.Context) (bool, error) { return true, nil }
	manager.set = func(context.Context, bool) error {
		t.Fatal("must not overwrite an externally owned sleep setting")
		return nil
	}
	if err := manager.Acquire(t.Context()); err == nil {
		t.Fatal("expected externally disabled sleep to be rejected")
	}
}

func TestSystemSleepLeaseManagerReconcilesOnlyItsOwnStaleMarker(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "sleep-prevention-owned")
	if err := os.WriteFile(marker, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := &systemSleepLeaseManager{marker: marker}
	called := false
	manager.set = func(_ context.Context, disabled bool) error {
		called = true
		if disabled {
			t.Fatal("stale reconciliation must re-enable sleep")
		}
		return nil
	}
	if err := manager.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("stale ownership marker did not trigger reconciliation")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale marker remained: %v", err)
	}
}

func TestSleepPreventionAPIControlsIndependentLease(t *testing.T) {
	server := newTestServer(t)
	runner := &fakeSleepPreventionRunner{}
	server.sleepPrevention = newSleepPreventionController(runner, server.configPath)

	response := performAuthorized(server, "PUT", "/api/v1/sleep-prevention", []byte(`{"enabled":true}`))
	if response.Code != 200 {
		t.Fatalf("enable status=%d body=%s", response.Code, response.Body.String())
	}
	var enabled SleepPreventionStatus
	if err := json.Unmarshal(response.Body.Bytes(), &enabled); err != nil {
		t.Fatal(err)
	}
	if !enabled.Enabled || !enabled.Active {
		t.Fatalf("enable response=%#v", enabled)
	}

	response = performAuthorized(server, "GET", "/api/v1/sleep-prevention", nil)
	if response.Code != 200 || !containsJSONBool(response.Body.Bytes(), "active", true) {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body.String())
	}

	response = performAuthorized(server, "PUT", "/api/v1/sleep-prevention", []byte(`{"enabled":false}`))
	if response.Code != 200 || !containsJSONBool(response.Body.Bytes(), "active", false) {
		t.Fatalf("disable status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSleepPreventionAPIReportsExternalOwnershipConflict(t *testing.T) {
	server := newTestServer(t)
	runner := &fakeSleepPreventionRunner{err: errSleepPreventionExternallyOwned}
	server.sleepPrevention = newSleepPreventionController(runner, server.configPath)

	response := performAuthorized(server, "PUT", "/api/v1/sleep-prevention", []byte(`{"enabled":true}`))
	if response.Code != 409 || !strings.Contains(response.Body.String(), "sleep_prevention_conflict") {
		t.Fatalf("conflict status=%d body=%s", response.Code, response.Body.String())
	}
}

func containsJSONBool(data []byte, key string, want bool) bool {
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	got, ok := value[key].(bool)
	return ok && got == want
}

type fakeSleepPreventionRunner struct {
	mu     sync.Mutex
	leases []*fakeSleepPreventionLease
	err    error
}

func (r *fakeSleepPreventionRunner) AcquireSleepPrevention(context.Context, string) (SleepPreventionLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	lease := &fakeSleepPreventionLease{done: make(chan error, 1)}
	r.leases = append(r.leases, lease)
	return lease, nil
}

func (r *fakeSleepPreventionRunner) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.leases)
}

func (r *fakeSleepPreventionRunner) LastLease() *fakeSleepPreventionLease {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.leases[len(r.leases)-1]
}

type fakeSleepPreventionLease struct {
	done chan error
	once sync.Once
}

func (l *fakeSleepPreventionLease) Done() <-chan error { return l.done }

func (l *fakeSleepPreventionLease) Close() error {
	l.once.Do(func() {
		l.done <- nil
		close(l.done)
	})
	return nil
}

func (l *fakeSleepPreventionLease) Fail(err error) {
	l.once.Do(func() {
		l.done <- err
		close(l.done)
	})
}
