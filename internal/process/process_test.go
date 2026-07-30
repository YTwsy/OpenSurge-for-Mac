package process

import (
	"errors"
	"os"
	"slices"
	"syscall"
	"testing"
)

func TestSignalErrMeansAlive(t *testing.T) {
	for _, err := range []error{
		nil,
		syscall.EPERM,
		os.ErrPermission,
		&os.SyscallError{Syscall: "kill", Err: syscall.EPERM},
	} {
		if !signalErrMeansAlive(err) {
			t.Fatalf("signalErrMeansAlive(%v) = false", err)
		}
	}

	for _, err := range []error{
		os.ErrProcessDone,
		syscall.ESRCH,
		errors.New("missing"),
	} {
		if signalErrMeansAlive(err) {
			t.Fatalf("signalErrMeansAlive(%v) = true", err)
		}
	}
}

func TestEnvironmentWithOverridesReplacesExistingValues(t *testing.T) {
	got := environmentWithOverrides(
		[]string{"PATH=/usr/bin", "SKIP_SYSTEM_IPV6_CHECK=0", "KEEP=yes"},
		map[string]string{"SKIP_SYSTEM_IPV6_CHECK": "1"},
	)
	if slices.Contains(got, "SKIP_SYSTEM_IPV6_CHECK=0") {
		t.Fatalf("environment kept replaced value: %#v", got)
	}
	for _, want := range []string{"PATH=/usr/bin", "KEEP=yes", "SKIP_SYSTEM_IPV6_CHECK=1"} {
		if !slices.Contains(got, want) {
			t.Fatalf("environment missing %q: %#v", want, got)
		}
	}
}
