package mihomo

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/process"
	"open-mihomo-gateway/internal/runtime"
)

func readinessManager(t *testing.T, api, script string) Manager {
	t.Helper()
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	cfg.Mihomo.Binary = filepath.Join(cfg.Runtime.Dir, "engine")
	cfg.Mihomo.APIAddr = api
	cfg.Transparent.Mode = config.TransparentModeOff
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Off
	m := New(cfg, runtime.NewPaths(cfg))
	if err := runtime.Ensure(m.paths); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.paths.MihomoConfig, []byte("dns: {enable: false}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Mihomo.Binary, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestStartContextWaitsForDelayedAPIAndKeepsDetachedProcess(t *testing.T) {
	readyAt := time.Now().Add(2400 * time.Millisecond)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if time.Now().Before(readyAt) {
			http.Error(w, "initializing", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"version":"delayed-test"}`))
	}))
	defer server.Close()
	m := readinessManager(t, server.URL, "exec sleep 30")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pid, err := m.StartContext(ctx)
	if pid > 0 {
		t.Cleanup(func() { _ = process.StopPID(pid, time.Second) })
	}
	if err != nil || pid <= 0 || time.Now().Before(readyAt) {
		t.Fatalf("delayed API start: pid=%d err=%v", pid, err)
	}
	cancel()
	if !process.IsAlive(pid) {
		t.Fatal("successful detached engine was tied to request cancellation")
	}
}

func TestWaitForAPIBoundedTimeoutRetainsLastProbe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	m := readinessManager(t, "http://"+addr, "exec sleep 30")
	started := time.Now()
	err = m.waitForAPI(t.Context(), os.Getpid(), 50*time.Millisecond)
	var failure *StartupError
	if !errors.As(err, &failure) || !errors.Is(err, context.DeadlineExceeded) || failure.Stage != "API" || failure.Reason != "timed out" || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("API timeout = %v", err)
	}
	if time.Since(started) > 250*time.Millisecond {
		t.Fatalf("probe exceeded stage budget: %s", time.Since(started))
	}
}

func TestStartContextEarlyExitPreservesDiagnosticAndCleansProcess(t *testing.T) {
	m := readinessManager(t, "http://127.0.0.1:1", "printf 'level=fatal msg=controlled-startup-failure\\n'\nexit 1")
	var stoppedPID int
	m.stopPID = func(pid int, timeout time.Duration) error {
		stoppedPID = pid
		return process.StopPID(pid, timeout)
	}
	started := time.Now()
	pid, err := m.StartContext(t.Context())
	var failure *StartupError
	if pid != 0 || !errors.As(err, &failure) || failure.Reason != "process exited" || !strings.Contains(err.Error(), "controlled-startup-failure") {
		t.Fatalf("early exit: pid=%d err=%v", pid, err)
	}
	if stoppedPID == 0 || process.IsAlive(stoppedPID) || time.Since(started) > time.Second {
		t.Fatalf("exited engine cleanup: pid=%d elapsed=%s", stoppedPID, time.Since(started))
	}
}

func TestStartContextCancellationCleansStartedProcess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	m := readinessManager(t, server.URL, "exec sleep 30")
	var stoppedPID int
	m.stopPID = func(pid int, timeout time.Duration) error {
		stoppedPID = pid
		return process.StopPID(pid, timeout)
	}
	ctx, cancel := context.WithCancel(t.Context())
	timer := time.AfterFunc(100*time.Millisecond, cancel)
	defer timer.Stop()
	defer cancel()
	pid, err := m.StartContext(ctx)
	var failure *StartupError
	if pid != 0 || !errors.As(err, &failure) || failure.Reason != "canceled" || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled start: pid=%d err=%v", pid, err)
	}
	if stoppedPID <= 0 || process.IsAlive(stoppedPID) {
		t.Fatalf("canceled startup left a live engine: %d", stoppedPID)
	}
}

func TestReadinessStagesHonorCancellation(t *testing.T) {
	m := readinessManager(t, "http://127.0.0.1:1", "exec sleep 30")
	for _, stage := range []string{"API", "DNS", "TUN", "IPv6 packet listener"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			var err error
			switch stage {
			case "API":
				err = m.waitForAPI(ctx, os.Getpid(), time.Second)
			case "DNS":
				err = m.waitForDNS(ctx, os.Getpid(), "ready.invalid", time.Second)
			case "TUN":
				err = m.waitForTUN(ctx, os.Getpid(), time.Second)
			default:
				err = m.waitForIPv6PacketListener(ctx, os.Getpid(), time.Second)
			}
			var failure *StartupError
			if !errors.As(err, &failure) || failure.Stage != stage || !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled %s: %v", stage, err)
			}
		})
	}
}

func TestFailedStartupReturnsOwnershipWhenCleanupFails(t *testing.T) {
	m := readinessManager(t, "http://127.0.0.1:1", "exec sleep 30")
	m.stopPID = func(int, time.Duration) error { return errors.New("controlled cleanup failure") }
	pid, err := m.failedStart(1234, context.Canceled)
	if pid != 1234 || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "controlled cleanup failure") {
		t.Fatalf("lost failed startup ownership: pid=%d err=%v", pid, err)
	}
}
