package controlapi

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/gateway"
)

func TestAutoMihomoRecoveryRestartsMissingProcessOncePerIncident(t *testing.T) {
	server := newTestServer(t)
	if err := server.store.SaveRecovery(RecoveryState{Stage: RecoveryGatewayActive, Required: true}); err != nil {
		t.Fatal(err)
	}
	status := gateway.Status{Gateway: "degraded", RuntimeState: "active", Mihomo: "stopped"}
	server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) { return status, nil }
	runner := &countingActionRunner{}
	server.runner = runner

	server.evaluateMihomoRecovery(t.Context())
	waitForRunnerCount(t, runner, 1)
	waitForRecoveryOperationCompletion(t, server)
	server.evaluateMihomoRecovery(t.Context())
	time.Sleep(30 * time.Millisecond)
	if got := runner.Count(); got != 1 {
		t.Fatalf("automatic restart count=%d, want one attempt for the incident", got)
	}
	if got := server.mihomoRecovery.snapshot(); got.State != mihomoRecoveryFailed || got.Error != "mihomo remained unhealthy after restart" {
		t.Fatalf("unhealthy status after successful restart command=%#v", got)
	}

	status = gateway.Status{Gateway: "running", RuntimeState: "active", Mihomo: "running (test)"}
	server.evaluateMihomoRecovery(t.Context())
	if got := server.mihomoRecovery.snapshot(); got.State != mihomoRecoveryRecovering {
		t.Fatalf("single healthy sample completed recovery=%#v", got)
	}
	server.evaluateMihomoRecovery(t.Context())
	status = gateway.Status{Gateway: "degraded", RuntimeState: "active", Mihomo: "stopped"}
	server.evaluateMihomoRecovery(t.Context())
	waitForRunnerCount(t, runner, 2)
}

func TestAutoMihomoRecoveryObservationErrorsDoNotResetIncident(t *testing.T) {
	server := newTestServer(t)
	if err := server.store.SaveRecovery(RecoveryState{Stage: RecoveryGatewayActive, Required: true}); err != nil {
		t.Fatal(err)
	}
	status := gateway.Status{Gateway: "degraded", RuntimeState: "active", Mihomo: "stopped"}
	var statusErr error
	server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) {
		return status, statusErr
	}
	runner := &countingActionRunner{}
	server.runner = runner

	server.evaluateMihomoRecovery(t.Context())
	waitForRunnerCount(t, runner, 1)
	waitForRecoveryOperationCompletion(t, server)

	statusErr = errors.New("temporary status read failure")
	server.evaluateMihomoRecovery(t.Context())
	server.evaluateMihomoRecovery(t.Context())
	if got := server.mihomoRecovery.snapshot(); got.State != mihomoRecoveryRecovering {
		t.Fatalf("unknown observations changed recovery state=%#v", got)
	}

	statusErr = nil
	server.evaluateMihomoRecovery(t.Context())
	time.Sleep(30 * time.Millisecond)
	if got := runner.Count(); got != 1 {
		t.Fatalf("observation errors allowed %d restarts for one incident", got)
	}
	if got := server.mihomoRecovery.snapshot(); got.State != mihomoRecoveryFailed {
		t.Fatalf("continued failure after unknown observations=%#v", got)
	}
}

func TestAutoMihomoRecoveryUnknownBreaksHealthyConfirmationSequence(t *testing.T) {
	controller := newMihomoRecoveryController()
	controller.beginManual()
	controller.finishManual(nil)

	controller.observeHealthy()
	controller.observeUnknown()
	controller.observeHealthy()
	if got := controller.snapshot(); got.State != mihomoRecoveryRecovering {
		t.Fatalf("non-consecutive healthy observations completed recovery=%#v", got)
	}
	controller.observeHealthy()
	if got := controller.snapshot(); got.State != mihomoRecoveryIdle {
		t.Fatalf("two fresh healthy observations did not complete recovery=%#v", got)
	}
}

func TestAutoMihomoRecoveryConfirmsControllerRefusalTwice(t *testing.T) {
	server := newTestServer(t)
	if err := server.store.SaveRecovery(RecoveryState{Stage: RecoveryClientValidated, Required: true}); err != nil {
		t.Fatal(err)
	}
	server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) {
		return gateway.Status{
			Gateway:      "running",
			RuntimeState: "active",
			Mihomo:       "running",
			MihomoError:  `Get "http://127.0.0.1:9090/version": dial tcp 127.0.0.1:9090: connect: connection refused`,
		}, nil
	}
	runner := &countingActionRunner{}
	server.runner = runner

	server.evaluateMihomoRecovery(t.Context())
	if got := runner.Count(); got != 0 {
		t.Fatalf("automatic restart count after first refusal=%d, want 0", got)
	}
	if got := server.mihomoRecovery.snapshot(); got.State != mihomoRecoveryObserving || got.Reason != mihomoFailureControllerRefused {
		t.Fatalf("recovery status after first refusal=%#v", got)
	}
	server.evaluateMihomoRecovery(t.Context())
	waitForRunnerCount(t, runner, 1)
}

func TestAutoMihomoRecoveryNeverUsesInterruptedRuntime(t *testing.T) {
	server := newTestServer(t)
	server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) {
		return gateway.Status{Gateway: "degraded", RuntimeState: "interrupted", Mihomo: "stopped"}, nil
	}
	runner := &countingActionRunner{}
	server.runner = runner

	server.evaluateMihomoRecovery(t.Context())
	if got := runner.Count(); got != 0 {
		t.Fatalf("interrupted runtime triggered %d automatic restarts", got)
	}
	if got := server.mihomoRecovery.snapshot(); got.State != mihomoRecoveryIdle {
		t.Fatalf("interrupted runtime recovery status=%#v", got)
	}
}

func TestAutoMihomoRecoveryFailureStopsUntilManualFallbackOrHealth(t *testing.T) {
	server := newTestServer(t)
	if err := server.store.SaveRecovery(RecoveryState{Stage: RecoveryGatewayActive, Required: true}); err != nil {
		t.Fatal(err)
	}
	server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) {
		return gateway.Status{Gateway: "degraded", RuntimeState: "active", Mihomo: "stopped"}, nil
	}
	runner := &countingActionRunner{err: errors.New("replacement did not become ready")}
	server.runner = runner

	server.evaluateMihomoRecovery(t.Context())
	waitForRunnerCount(t, runner, 1)
	waitForRecoveryState(t, server, mihomoRecoveryFailed)
	server.evaluateMihomoRecovery(t.Context())
	time.Sleep(30 * time.Millisecond)
	if got := runner.Count(); got != 1 {
		t.Fatalf("failed incident retried automatically %d times", got)
	}
	status := server.mihomoRecovery.snapshot()
	if status.Reason != mihomoFailureProcessMissing || status.Error != "replacement did not become ready" {
		t.Fatalf("failed recovery status=%#v", status)
	}
}

func TestAutoMihomoRecoveryWaitsForLifecycleOperation(t *testing.T) {
	server := newTestServer(t)
	if err := server.store.SaveRecovery(RecoveryState{Stage: RecoveryGatewayActive, Required: true}); err != nil {
		t.Fatal(err)
	}
	server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) {
		return gateway.Status{Gateway: "degraded", RuntimeState: "active", Mihomo: "stopped"}, nil
	}
	runner := &countingActionRunner{}
	server.runner = runner
	server.lifecycleMu.Lock()
	server.evaluateMihomoRecovery(t.Context())
	if got := runner.Count(); got != 0 {
		t.Fatalf("busy lifecycle triggered %d automatic restarts", got)
	}
	server.lifecycleMu.Unlock()
	server.evaluateMihomoRecovery(t.Context())
	waitForRunnerCount(t, runner, 1)
}

type countingActionRunner struct {
	mu    sync.Mutex
	count int
	err   error
}

func (r *countingActionRunner) Run(context.Context, string, string) error {
	r.mu.Lock()
	r.count++
	err := r.err
	r.mu.Unlock()
	return err
}

func (r *countingActionRunner) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

func waitForRunnerCount(t *testing.T, runner *countingActionRunner, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runner.Count() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("runner count=%d, want at least %d", runner.Count(), want)
}

func waitForRecoveryState(t *testing.T, server *Server, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if server.mihomoRecovery.snapshot().State == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("recovery status=%#v, want state %q", server.mihomoRecovery.snapshot(), want)
}

func waitForRecoveryOperationCompletion(t *testing.T, server *Server) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		server.mihomoRecovery.mu.Lock()
		active := server.mihomoRecovery.operationActive
		server.mihomoRecovery.mu.Unlock()
		if !active {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("mihomo recovery operation did not complete")
}
