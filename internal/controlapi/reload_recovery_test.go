package controlapi

import (
	"bytes"
	"context"
	"encoding/json"
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
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/runtime"
)

func healthyReloadStatus(context.Context, config.Config) (gateway.Status, error) {
	return gateway.Status{Gateway: "running", RuntimeState: "active", DHCP: "running", Mihomo: "running", TUN: "ready", PFAnchor: "loaded", Forwarding: "enabled", IPv4Takeover: "ready"}, nil
}

type transactionConfigurationRunner struct {
	fakeConfigurationRunner
	deps profileApplyDeps
}

func (r transactionConfigurationRunner) ApplyProfile(ctx context.Context, path, revision string, payload []byte, source, overlay string) (ProfileApplyResult, error) {
	return applyProfile(ctx, path, revision, payload, source, overlay, r.deps)
}

func TestSourceApplyFailureReconcilesFinalGateway(t *testing.T) {
	for _, tc := range []struct {
		name          string
		phase         gateway.ReloadPhase
		residual      bool
		liveCandidate bool
		restartErr    bool
		unhealthy     bool
		wantStage     string
	}{
		{name: "previous gateway restored", phase: gateway.ReloadStart},
		{name: "previous gateway also fails", phase: gateway.ReloadStart, restartErr: true, wantStage: RecoveryRouterDHCPDisabledConfirmed},
		{name: "candidate leaves residual runtime", phase: gateway.ReloadStart, residual: true},
		{name: "healthy candidate survives failed rollback", phase: gateway.ReloadStart, residual: true, liveCandidate: true},
		{name: "partial stop failure", phase: gateway.ReloadStop, residual: true},
		{name: "candidate validation failure", phase: gateway.ReloadValidation},
		{name: "previous start returns but health degrades", phase: gateway.ReloadStart, unhealthy: true},
	} {
		for _, before := range []string{RecoveryGatewayActive, RecoveryClientValidated, RecoveryClientValidationSkipped} {
			t.Run(tc.name+"/"+before, func(t *testing.T) {
				server := newTestServer(t)
				oldSource, err := server.importReader("old", "mihomo_profile", "file:old.yaml", strings.NewReader("rules:\n  - MATCH,DIRECT\n"))
				if err != nil {
					t.Fatal(err)
				}
				candidate, err := server.importReader("candidate", "mihomo_profile", "file:candidate.yaml", strings.NewReader("rules:\n  - MATCH,REJECT\n"))
				if err != nil {
					t.Fatal(err)
				}
				cfg, err := config.Load(server.configPath)
				if err != nil {
					t.Fatal(err)
				}
				cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
				cfg.Mihomo.Profile = filepath.Join(filepath.Dir(server.configPath), "old-profile.yaml")
				cfg.Mihomo.ProfileSourceDigest = oldSource.Digest
				if err := writeAtomic(cfg.Mihomo.Profile, []byte("rules:\n  - MATCH,DIRECT\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				original := []byte(config.Render(cfg))
				if err := writeAtomic(server.configPath, original, 0o600); err != nil {
					t.Fatal(err)
				}
				paths := runtime.NewPaths(cfg)
				if err := runtime.Ensure(paths); err != nil {
					t.Fatal(err)
				}
				state := runtime.State{StartedAt: time.Now(), PIDDNSMasq: os.Getpid(), PIDMihomo: os.Getpid(), ProfileDigest: oldSource.Digest}
				if err := runtime.SaveState(paths.StateFile, state); err != nil {
					t.Fatal(err)
				}
				if err := server.store.SaveRecovery(RecoveryState{Stage: before, Required: true, ClientValidationSkipped: before == RecoveryClientValidationSkipped, RecoveryNotes: "existing evidence"}); err != nil {
					t.Fatal(err)
				}
				status, _ := healthyReloadStatus(t.Context(), cfg)
				server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) { return status, nil }
				started := false
				candidatePath := ""
				server.configRunner = transactionConfigurationRunner{deps: profileApplyDeps{
					geteuid:  func() int { return 0 },
					validate: func(config.Config) error { return nil },
					stateExists: func(config.Config) (bool, error) {
						_, exists, err := runtime.LoadState(paths.StateFile)
						return exists, err
					},
					status: server.gatewayStatus,
					reload: func(_ context.Context, candidateCfg config.Config) error {
						candidatePath = candidateCfg.Mihomo.Profile
						if tc.residual {
							if tc.liveCandidate {
								residualState := state
								residualState.ProfileDigest, _ = config.MihomoProfileDigest(candidateCfg)
								if err := runtime.SaveState(paths.StateFile, residualState); err != nil {
									return err
								}
							} else {
								status.Gateway = "degraded"
							}
						} else if tc.phase == gateway.ReloadStart {
							if err := runtime.RemoveState(paths.StateFile); err != nil {
								return err
							}
							status = gateway.Status{Gateway: "stopped", RuntimeState: "none"}
						}
						return &gateway.ReloadError{Phase: tc.phase, Err: errors.New("controlled candidate failure")}
					},
					start: func(ctx context.Context, previous config.Config) error {
						started = true
						if previous.Mihomo.Profile != cfg.Mihomo.Profile {
							t.Error("restarted candidate instead of previous desired config")
						}
						if tc.restartErr {
							return errors.New("controlled previous startup failure")
						}
						status, _ = healthyReloadStatus(ctx, previous)
						if tc.unhealthy {
							status.MihomoError = "controller unreachable"
						}
						return runtime.SaveState(paths.StateFile, state)
					},
				}}
				request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:61767/api/v1/sources/"+candidate.ID+"/apply", nil)
				request.Header.Set("Authorization", "Bearer "+server.token)
				request.Header.Set("If-Match", fileDigest(server.configPath))
				request.Header.Set(operationIDHeader, "rollback-test")
				response := httptest.NewRecorder()
				server.Handler().ServeHTTP(response, request)
				if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "controlled candidate failure") {
					t.Fatalf("apply status=%d body=%s", response.Code, response.Body.String())
				}
				if started != (tc.phase == gateway.ReloadStart && !tc.residual) {
					t.Fatalf("unexpected previous startup: %t", started)
				}
				current, err := os.ReadFile(server.configPath)
				if err != nil || !bytes.Equal(current, original) {
					t.Fatalf("previous desired config not restored: %v", err)
				}
				recovery, err := server.store.Recovery()
				wantStage := tc.wantStage
				if wantStage == "" {
					wantStage = before
				}
				if err != nil || recovery.Stage != wantStage || !recovery.Required || recovery.ClientValidationSkipped != (before == RecoveryClientValidationSkipped) || !strings.Contains(recovery.RecoveryNotes, "existing evidence") || !strings.Contains(recovery.RecoveryNotes, "controlled candidate failure") {
					t.Fatalf("recovery=%+v err=%v", recovery, err)
				}
				if tc.residual || tc.unhealthy || tc.restartErr {
					if strings.Contains(response.Body.String(), "previous running gateway preserved or restored") {
						t.Fatalf("unhealthy runtime reported restored: %s", response.Body.String())
					}
					if tc.residual || tc.unhealthy {
						if _, err := os.Stat(candidatePath); err != nil {
							t.Fatalf("uncertain runtime lost candidate recovery material: %v", err)
						}
					}
					if tc.liveCandidate {
						listed := server.decorateSourceStates([]Source{oldSource, candidate})
						if !listed[0].Desired || listed[0].Applied || !strings.Contains(recovery.RecoveryNotes, "previous applied configuration is unconfirmed") {
							t.Fatalf("candidate residual was called restored: sources=%+v recovery=%+v", listed, recovery)
						}
					}
				} else {
					listed := server.decorateSourceStates([]Source{oldSource, candidate})
					if !listed[0].Desired || !listed[0].Applied || listed[1].Desired || listed[1].Applied {
						t.Fatalf("desired/applied drift after restoration: %+v", listed)
					}
					if recovery.Stage == RecoveryRouterDHCPDisabledConfirmed || strings.Contains(recovery.RecoveryNotes, "retry start") {
						t.Fatalf("healthy gateway received startup guidance: %+v", recovery)
					}
				}
				op, err := server.store.Operation("rollback-test")
				if err != nil || op.State != "failed" || !strings.Contains(op.Error, "controlled candidate failure") {
					t.Fatalf("apply failure lost: %+v err=%v", op, err)
				}
			})
		}
	}
}

func TestProfileRollbackContinuesAfterCancellation(t *testing.T) {
	path, original := writeProfileApplyTestConfig(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	checks := 0
	deps := profileApplyDeps{
		geteuid: func() int { return 0 }, validate: func(config.Config) error { return nil }, status: healthyReloadStatus,
		stateExists: func(config.Config) (bool, error) { checks++; return checks == 1, nil },
		reload: func(context.Context, config.Config) error {
			cancel()
			return &gateway.ReloadError{Phase: gateway.ReloadStart, Err: context.Canceled}
		},
		start: func(ctx context.Context, _ config.Config) error {
			if ctx.Err() != nil {
				t.Error("cleanup inherited canceled apply context")
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Error("cleanup has no deadline")
			}
			return ctx.Err()
		},
	}
	result, err := applyProfile(ctx, path, fileDigest(path), profileApplyFixture(), profileApplySourceDigest(), "", deps)
	if !errors.Is(err, context.Canceled) || result.Rollback == nil || !result.Rollback.RestartAttempted || !result.Rollback.PreviousGatewayReady || result.Rollback.GatewayState != "running" {
		t.Fatalf("canceled apply rollback: %+v err=%v", result, err)
	}
	current, _ := os.ReadFile(path)
	if !bytes.Equal(current, original) {
		t.Fatal("canceled apply did not restore desired config")
	}
}

func TestReloadRecoveryDoesNotInferHealthFromStateFile(t *testing.T) {
	for _, kind := range []string{"unreadable", "interrupted", "status unavailable", "stale stopped observation"} {
		t.Run(kind, func(t *testing.T) {
			server := newTestServer(t)
			cfg, _ := config.LoadRuntime(server.configPath)
			paths := runtime.NewPaths(cfg)
			if err := runtime.Ensure(paths); err != nil {
				t.Fatal(err)
			}
			if err := runtime.SaveState(paths.StateFile, runtime.State{BootSessionID: "previous-boot"}); err != nil {
				t.Fatal(err)
			}
			if kind == "unreadable" {
				if err := os.WriteFile(paths.StateFile, []byte("broken"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) {
				if kind == "status unavailable" || kind == "unreadable" {
					return gateway.Status{}, errors.New("status unavailable")
				}
				if kind == "stale stopped observation" {
					return gateway.Status{Gateway: "stopped", RuntimeState: "none"}, nil
				}
				return gateway.Status{Gateway: "degraded", RuntimeState: "interrupted"}, nil
			}
			server.recordReloadRecoveryFailure(cfg.Gateway.Mode, RecoveryState{Stage: RecoveryClientValidated}, &gateway.ReloadError{Phase: gateway.ReloadStart, Err: errors.New("candidate failure")}, nil)
			recovery, _ := server.store.Recovery()
			if recovery.Stage != RecoveryClientValidated || !recovery.Required || strings.Contains(recovery.RecoveryNotes, "health confirmed") || strings.Contains(recovery.RecoveryNotes, "retry start") || !strings.Contains(recovery.RecoveryNotes, "candidate failure") {
				t.Fatalf("unsafe recovery: %+v", recovery)
			}
		})
	}
}

func TestHelperFailureRetainsReloadPhaseAndRollback(t *testing.T) {
	dir, err := os.MkdirTemp("", "os-rollback-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "helper.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		var request HelperRequest
		if err := json.NewDecoder(conn).Decode(&request); err != nil {
			done <- err
			return
		}
		done <- json.NewEncoder(conn).Encode(HelperResponse{Error: "controlled transaction failure", ReloadPhase: gateway.ReloadStart, Rollback: &ConfigurationRollback{ConfigRestored: true, RestartAttempted: true, GatewayState: "running", PreviousGatewayReady: true}})
	}()
	result, err := (HelperClient{SocketPath: path}).ApplyProfile(t.Context(), "/unused", "revision", []byte("profile"), "", "")
	var failure *gateway.ReloadError
	if !errors.As(err, &failure) || failure.Phase != gateway.ReloadStart || err.Error() != "controlled transaction failure" || result.Rollback == nil || !result.Rollback.ConfigRestored || !result.Rollback.PreviousGatewayReady || result.Rollback.GatewayState != "running" {
		t.Fatalf("lost Helper failure metadata: %+v err=%v", result, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
