package gateway

import (
	"context"
	"testing"

	"open-mihomo-gateway/internal/runtime"
)

// A NAS reboot changes both the host boot_id and the container network
// namespace. Persistent desired state must survive that boundary, stale runtime
// must be cleaned without signalling old PIDs, and the gateway must be started
// again from the persisted configuration.
func TestRecoverAfterQNAPHostRebootRestartsDesiredGateway(t *testing.T) {
	cfg := qnapRecoveryTestConfig(t)
	saveInterruptedQNAPRuntime(t, cfg)
	paths := runtime.NewPaths(cfg)
	if err := runtime.SaveGatewayDesiredState(runtime.GatewayDesiredStatePath(paths.Dir), true); err != nil {
		t.Fatal(err)
	}

	backend := &fakeBackend{}
	dhcpManager := &fakeDHCP{startPID: 3002}
	mihomoManager := &fakeMihomo{startPID: 3001}
	manager := qnapRecoveryTestManager(t, cfg, backend, dhcpManager, mihomoManager)
	manager.deps.currentBoot = func() (runtime.BootSession, error) {
		return runtime.BootSession{ID: "host-boot-after-reboot", NetworkNamespace: "net:[after-reboot]"}, nil
	}

	recovered, err := manager.RecoverAfterContainerRestart(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !recovered {
		t.Fatal("QNAP reboot recovery reported no data-plane restart")
	}
	if !mihomoManager.startCalled || !dhcpManager.startCalled {
		t.Fatalf("gateway services were not restored after host reboot: mihomo=%v dns=%v", mihomoManager.startCalled, dhcpManager.startCalled)
	}
	if mihomoManager.stopCalled || dhcpManager.stopCalled {
		t.Fatal("host reboot recovery signalled stale process IDs from the previous boot")
	}
	if backend.restoreCalls != 1 {
		t.Fatalf("stale network snapshot restore calls = %d, want 1", backend.restoreCalls)
	}
	if backend.routingCalls != 1 {
		t.Fatalf("fresh policy routing setup calls = %d, want 1", backend.routingCalls)
	}

	desired, exists, err := runtime.LoadGatewayDesiredState(runtime.GatewayDesiredStatePath(paths.Dir))
	if err != nil || !exists || !desired.Running {
		t.Fatalf("running intent after QNAP reboot = %#v exists=%v err=%v", desired, exists, err)
	}
	state, exists, err := runtime.LoadState(paths.StateFile)
	if err != nil || !exists {
		t.Fatalf("fresh runtime state after QNAP reboot: exists=%v err=%v", exists, err)
	}
	if state.BootSessionID != "host-boot-after-reboot" || state.PIDMihomo != 3001 || state.PIDDNSMasq != 3002 {
		t.Fatalf("fresh runtime state after QNAP reboot = %#v", state)
	}
}
