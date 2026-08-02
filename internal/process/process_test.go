package process

import (
	"errors"
	"os"
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

func TestFingerprintMatchesCurrentProcessAndRejectsDifferentIdentity(t *testing.T) {
	fingerprint, err := Fingerprint(os.Getpid())
	if errors.Is(err, os.ErrPermission) {
		t.Skipf("process inspection is blocked by the test sandbox: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint == "" {
		t.Fatal("current process fingerprint is empty")
	}
	matches, err := MatchesFingerprint(os.Getpid(), fingerprint)
	if err != nil || !matches {
		t.Fatalf("matching fingerprint = %v, %v", matches, err)
	}
	matches, err = MatchesFingerprint(os.Getpid(), fingerprint+" changed")
	if err != nil || matches {
		t.Fatalf("different fingerprint = %v, %v", matches, err)
	}
}

func TestFingerprintTreatsMissingProcessAsAbsent(t *testing.T) {
	const impossiblePID = 2_000_000_000
	fingerprint, err := Fingerprint(impossiblePID)
	if err != nil || fingerprint != "" {
		t.Fatalf("missing process fingerprint = %q, %v", fingerprint, err)
	}
}
