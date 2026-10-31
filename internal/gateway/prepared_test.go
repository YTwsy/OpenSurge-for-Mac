package gateway

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

func TestStartAndStopRefuseUnreleasedPreparedEngine(t *testing.T) {
	for _, action := range []string{"start", "stop"} {
		t.Run(action, func(t *testing.T) {
			cfg := config.Default()
			cfg.Gateway.Interface = "lan0"
			cfg.Gateway.UpstreamInterface = "wan0"
			cfg.Runtime.Dir = t.TempDir()
			cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
			manager := Manager{cfg: cfg, paths: runtime.NewPaths(cfg), deps: gatewayDeps{
				geteuid: func() int { return 0 }, loadState: runtime.LoadState,
				stopPrepared: func(config.Config) error { return errors.New("prepared process still owns cache") },
			}}
			var err error
			if action == "start" {
				err = manager.Start(context.Background())
			} else {
				err = manager.Stop(context.Background())
			}
			if err == nil || !strings.Contains(err.Error(), "prepared process still owns cache") {
				t.Fatalf("%s proceeded before prepared process exit: %v", action, err)
			}
		})
	}
}

func TestStopReleasesPreparedEngineWithoutGatewayState(t *testing.T) {
	cfg := config.Default()
	cfg.Runtime.Dir = t.TempDir()
	cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
	stops := 0
	manager := Manager{cfg: cfg, paths: runtime.NewPaths(cfg), deps: gatewayDeps{
		geteuid: func() int { return 0 }, loadState: runtime.LoadState, removeState: runtime.RemoveState,
		stopPrepared: func(config.Config) error { stops++; return nil },
		newPF:        func(config.Config, runtime.Paths) pfService { return &fakePF{} },
		newSysctl:    func() sysctlService { return &fakeSysctl{} },
	}}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stops != 1 {
		t.Fatalf("stopped gateway did not reconcile prepared engine: calls=%d", stops)
	}
}

func TestStopCleansGatewayDespiteUnresolvedPreparedIdentity(t *testing.T) {
	manager, _, engine, events := dnsLifecycleFixture(t)
	if err := manager.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	preparedErr := errors.New("prepared engine state lacks a process identity")
	manager.deps.stopPrepared = func(config.Config) error { return preparedErr }
	firewall, forwarding := &fakePF{}, &fakeSysctl{}
	manager.deps.newPF = func(config.Config, runtime.Paths) pfService { return firewall }
	manager.deps.newSysctl = func() sysctlService { return forwarding }
	*events = nil
	if err := manager.Stop(t.Context()); !errors.Is(err, preparedErr) {
		t.Fatalf("lost prepared recovery error: %v", err)
	}
	for _, event := range []string{"dns-restore", "dhcp-stop", "mihomo-stop"} {
		if !slices.Contains(*events, event) {
			t.Fatalf("prepared recovery failure blocked %s: %v", event, *events)
		}
	}
	if !engine.stopCalled {
		t.Fatal("independently tracked gateway core was left running")
	}
	if !firewall.unloadCalled || forwarding.restoreValue != "0" {
		t.Fatal("prepared identity failure blocked PF or forwarding cleanup")
	}
	if _, exists, err := runtime.LoadState(manager.paths.StateFile); err != nil || !exists {
		t.Fatalf("unfinished cleanup lost its retry state: exists=%t, %v", exists, err)
	}
}

func TestStopValidatesPFAnchorOnlyWhenCurrentRuntimeOwnsOne(t *testing.T) {
	for _, scenario := range []string{"owned-current-boot", "not-owned", "previous-boot"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := gatewayTestConfig()
			cfg.Runtime.Dir = t.TempDir()
			cfg.PF.AnchorName = " "
			paths := runtime.NewPaths(cfg)
			state := runtime.State{BootSessionID: "current", IPForwardingBefore: "0", PFAnchorLoaded: scenario != "not-owned"}
			if scenario == "previous-boot" {
				state.BootSessionID = "previous"
			}
			if err := runtime.SaveState(paths.StateFile, state); err != nil {
				t.Fatal(err)
			}
			firewall, forwarding := &fakePF{}, &fakeSysctl{}
			manager := Manager{cfg: cfg, paths: paths, deps: gatewayDeps{
				geteuid: func() int { return 0 }, loadState: runtime.LoadState, removeState: runtime.RemoveState,
				stopPrepared: func(config.Config) error { return nil },
				currentBoot:  func() (runtime.BootSession, error) { return runtime.BootSession{ID: "current"}, nil },
				newDHCP:      func(config.Config, runtime.Paths) dhcpService { return &fakeDHCP{} },
				newMihomo:    func(config.Config, runtime.Paths) mihomoService { return &fakeMihomo{} },
				newPF:        func(config.Config, runtime.Paths) pfService { return firewall },
				newSysctl:    func() sysctlService { return forwarding },
			}}
			err := manager.Stop(t.Context())
			wantErr := scenario == "owned-current-boot"
			if (err != nil) != wantErr || wantErr && !strings.Contains(err.Error(), "pf.anchor_name") {
				t.Fatalf("stop=%v, wantError=%t", err, wantErr)
			}
			if firewall.unloadCalled {
				t.Fatal("attempted PF unload without a known anchor target")
			}
			if scenario != "previous-boot" && forwarding.restoreValue != "0" {
				t.Fatal("PF target error blocked independent forwarding restoration")
			}
			if scenario == "previous-boot" && forwarding.restoreValue != "" {
				t.Fatal("old runtime changed this boot's forwarding")
			}
			if _, exists, loadErr := runtime.LoadState(paths.StateFile); loadErr != nil || exists != wantErr {
				t.Fatalf("retry state exists=%t, %v", exists, loadErr)
			}
		})
	}
}

func TestCandidateStartCommitsAfterFinalValidation(t *testing.T) {
	for _, tt := range []struct {
		name, cancelPhase                        string
		validationErr, commitErr, startErr       error
		wantCommitted, wantForwarding, wantStart bool
	}{
		{name: "success", wantCommitted: true, wantForwarding: true, wantStart: true},
		{name: "invalid", validationErr: errors.New("invalid core config")},
		{name: "commit failure", commitErr: errors.New("cannot save desired")},
		{name: "cancel before commit", cancelPhase: "saving_config"},
		{name: "cancel after commit", cancelPhase: "saving_runtime", wantCommitted: true},
		{name: "cancel after forwarding", cancelPhase: "starting_mihomo", wantCommitted: true, wantForwarding: true},
		{name: "network start failure", startErr: errors.New("core could not start"), wantCommitted: true, wantForwarding: true, wantStart: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Gateway.Interface, cfg.Gateway.UpstreamInterface = "lan0", "wan0"
			cfg.Runtime.Dir = t.TempDir()
			cfg.Mihomo.Config = filepath.Join(cfg.Runtime.Dir, "mihomo.yaml")
			var events []string
			core := &fakeMihomo{validateErr: tt.validationErr, startErr: tt.startErr, events: &events}
			dns := &fakeDHCP{}
			firewall := &fakePF{loaded: true}
			forwarding := &fakeSysctl{current: "0"}
			manager := Manager{cfg: cfg, paths: runtime.NewPaths(cfg), deps: gatewayDeps{
				geteuid: func() int { return 0 }, loadState: runtime.LoadState,
				saveState: runtime.SaveState, removeState: runtime.RemoveState, ensure: runtime.Ensure,
				stopPrepared:    func(config.Config) error { events = append(events, "stop-prepared"); return nil },
				newMihomo:       func(config.Config, runtime.Paths) mihomoService { return core },
				newDHCP:         func(config.Config, runtime.Paths) dhcpService { return dns },
				newPF:           func(config.Config, runtime.Paths) pfService { return firewall },
				newSysctl:       func() sysctlService { return forwarding },
				interfaces:      func() ([]net.Interface, error) { return []net.Interface{{Name: "lan0"}}, nil },
				interfaceByName: func(name string) (*net.Interface, error) { return &net.Interface{Name: name}, nil },
				interfaceAddrs: func(*net.Interface) ([]net.Addr, error) {
					return []net.Addr{&net.IPNet{IP: net.ParseIP(cfg.Gateway.LANIP), Mask: net.CIDRMask(24, 32)}}, nil
				},
				currentBoot: func() (runtime.BootSession, error) { return runtime.BootSession{ID: "test-boot"}, nil },
				now:         time.Now,
			}}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ctx = WithProgress(ctx, func(p Progress) {
				if p.Phase == tt.cancelPhase {
					cancel()
				}
			})
			committed := false
			err := manager.StartCandidateLocked(ctx, func() error {
				if !slices.Equal(events, []string{"stop-prepared", "mihomo-write", "mihomo-validate"}) {
					t.Fatalf("candidate was not validated exactly once after prepared exit: %v", events)
				}
				if forwarding.enableCalled || core.startCalled || firewall.loadCalled {
					t.Fatal("network takeover preceded commit")
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if tt.commitErr != nil {
					return tt.commitErr
				}
				committed = true
				return nil
			})
			if (err == nil) != (tt.name == "success") {
				t.Fatalf("start error=%v", err)
			}
			if tt.cancelPhase != "" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
			if committed != tt.wantCommitted || forwarding.enableCalled != tt.wantForwarding || core.startCalled != tt.wantStart {
				t.Fatalf("committed=%t forwarding=%t core start=%t", committed, forwarding.enableCalled, core.startCalled)
			}
			if err != nil {
				if _, exists, stateErr := runtime.LoadState(manager.paths.StateFile); stateErr != nil || exists {
					t.Fatalf("failed candidate retained runtime: exists=%t err=%v", exists, stateErr)
				}
				if tt.wantForwarding && forwarding.restoreValue != "0" {
					t.Fatal("forwarding was not restored")
				}
			}
		})
	}
}
