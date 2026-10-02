package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/runtime"
)

func healthyGatewayStatus() gateway.Status {
	return gateway.Status{Gateway: "running", RuntimeState: "active", DHCP: "running", Mihomo: "running", TUN: "ready", PFAnchor: "loaded", Forwarding: "enabled", IPv4Takeover: "ready", IPv6Takeover: "disabled"}
}

func TestGatewayPresentationSeparatesOperationsFromRuntimeFailures(t *testing.T) {
	now := time.Now()
	for _, test := range []struct{ kind, phase, want string }{
		{"start", "starting_mihomo", "starting"}, {"reload", "stopping_dns", "reloading"},
		{"stop", "restoring_network", "stopping"}, {"restart-mihomo", "starting_mihomo", "recovering"},
		{"apply-profile", "validating_config", "changing"}, {"apply-tailscale", "starting_mihomo", "changing"},
		{"reload", "rolling_back", "rolling_back"}, {"apply-profile", "restoring_config", "rolling_back"},
	} {
		t.Run(test.kind+"/"+test.phase, func(t *testing.T) {
			op := newOperation("test", test.kind)
			op.Phase = test.phase
			for _, raw := range []string{"stopped", "degraded", "running"} {
				status := healthyGatewayStatus()
				status.Gateway, status.DHCP = raw, "stopped"
				view := deriveGatewayPresentation(status, nil, RecoveryState{Required: true, Stage: RecoveryRouterDHCPDisabledConfirmed}, gatewayActivitySnapshot{known: true, operation: &op}, true, now)
				if view.State != test.want || view.Phase != test.phase || !view.Busy {
					t.Fatalf("%s: %+v", raw, view)
				}
			}
		})
	}
}

func TestGatewayPresentationReportsEvidenceAndUnknownSeparately(t *testing.T) {
	for _, test := range []struct {
		name, want, reason string
		change             func(*gateway.Status)
	}{
		{"healthy", "running", "", func(*gateway.Status) {}},
		{"dns exited", "degraded", "dns_stopped", func(s *gateway.Status) { s.DHCP = "stopped" }},
		{"engine exited", "degraded", "mihomo_stopped", func(s *gateway.Status) { s.Mihomo = "stopped" }},
		{"tun disabled", "degraded", "tun_failed", func(s *gateway.Status) { s.TUN = "failed" }},
		{"pf missing despite running processes", "degraded", "pf_unloaded", func(s *gateway.Status) { s.PFAnchor = "unloaded"; s.IPv4Takeover = "failed" }},
		{"forwarding disabled", "degraded", "forwarding_disabled", func(s *gateway.Status) { s.Forwarding = "disabled" }},
		{"forwarding unreadable", "unknown", "status_unavailable", func(s *gateway.Status) { s.Forwarding = "unknown"; s.IPv4Takeover = "failed" }},
		{"tun unreadable", "unknown", "status_unavailable", func(s *gateway.Status) { s.TUN = "unknown" }},
		{"controller unreachable", "unknown", "status_unavailable", func(s *gateway.Status) { s.MihomoError = "connection refused" }},
		{"ipv6 failed", "degraded", "ipv6_failed", func(s *gateway.Status) { s.IPv6Packet = "failed" }},
		{"ipv6 waiting is healthy", "running", "", func(s *gateway.Status) { s.IPv6Takeover = "waiting" }},
		{"boot interrupted", "interrupted", "runtime_interrupted", func(s *gateway.Status) { s.RuntimeState = "interrupted"; s.Gateway = "degraded" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			status := healthyGatewayStatus()
			test.change(&status)
			view := deriveGatewayPresentation(status, nil, RecoveryState{}, gatewayActivitySnapshot{}, false, time.Now())
			if view.State != test.want || view.Reason != test.reason {
				t.Fatalf("%+v", view)
			}
		})
	}
	view := deriveGatewayPresentation(gateway.Status{}, errors.New("cannot read state"), RecoveryState{}, gatewayActivitySnapshot{}, false, time.Now())
	if view.State != "unknown" {
		t.Fatalf("unreadable status = %+v", view)
	}
}

func TestGatewayPresentationDoesNotPromoteSavedConfigOrDiagnosticFailure(t *testing.T) {
	server := newTestServer(t)
	server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) { return healthyGatewayStatus(), nil }
	// An explicit failed Doctor run for this same config is advisory.
	server.doctor.revision = server.currentDoctorRevision()
	server.doctor.state = doctorRunFailed
	for _, path := range []string{"/api/v1/overview", "/api/v1/menubar"} {
		response := performAuthorized(server, http.MethodGet, path, nil)
		var data MenuBarStatus
		if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if data.Presentation.State != "running" || !data.Drift || !data.Presentation.ConfigPending || !data.Presentation.DiagnosisWarning {
			t.Fatalf("%s: %s", path, response.Body.String())
		}
	}
	op := newOperation("save", "save-device-policy")
	server.gatewayActivity.update(op)
	view := deriveGatewayPresentation(healthyGatewayStatus(), nil, RecoveryState{}, server.gatewayActivity.snapshot(), true, time.Now())
	if view.State != "running" {
		t.Fatalf("save misreported as a gateway transition: %+v", view)
	}
}

func TestGatewayPresentationUsesLiveOwnershipAndBoundedProgress(t *testing.T) {
	server := newTestServer(t)
	server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) { return healthyGatewayStatus(), nil }
	cfg, err := config.LoadRuntime(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	op := newOperation("old-start", "start")
	op.CreatedAt = time.Now().Add(-2 * gatewayOperationTimeout)
	if err := server.store.CreateOperation(op); err != nil {
		t.Fatal(err)
	}
	_, _, _, view := server.observeGateway(context.Background(), cfg)
	if view.State != "running" || view.Busy {
		t.Fatalf("orphan history looked live: %+v", view)
	}
	server.gatewayActivity.update(op)
	_, _, _, view = server.observeGateway(context.Background(), cfg)
	if view.State != "unknown" || view.Reason != "operation_unconfirmed" || !view.Busy {
		t.Fatalf("stale live operation: %+v", view)
	}
	server.finishOperation(&op, errors.New("candidate rejected"))
	_, _, _, view = server.observeGateway(context.Background(), cfg)
	if view.State != "running" || view.Busy {
		t.Fatalf("failed change hid the still running gateway: %+v", view)
	}
	lock, err := runtime.AcquireLifecycleLock(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Release() })
	_, _, _, view = server.observeGateway(context.Background(), cfg)
	if view.State != "changing" || !view.Busy {
		t.Fatalf("external lifecycle ignored: %+v", view)
	}
}

func TestGatewayPresentationRechecksSampleCrossingOperationCompletion(t *testing.T) {
	server := newTestServer(t)
	op := newOperation("reload", "reload")
	server.gatewayActivity.update(op)
	calls := 0
	server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) {
		calls++
		if calls == 1 {
			server.finishOperation(&op, nil)
			return gateway.Status{Gateway: "degraded", DHCP: "stopped"}, nil
		}
		return healthyGatewayStatus(), nil
	}
	cfg, err := config.LoadRuntime(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, view := server.observeGateway(context.Background(), cfg)
	if calls != 2 || view.State != "running" {
		t.Fatalf("calls=%d view=%+v", calls, view)
	}
}

func TestGatewayPresentationPreservesRecoveryAfterFailedStartup(t *testing.T) {
	view := deriveGatewayPresentation(gateway.Status{Gateway: "stopped"}, nil, RecoveryState{Required: true, Stage: RecoveryRouterDHCPDisabledConfirmed}, gatewayActivitySnapshot{}, false, time.Now())
	if view.State != "recovery" || view.Reason != RecoveryRouterDHCPDisabledConfirmed {
		t.Fatalf("%+v", view)
	}
}

func TestGatewayPresentationRechecksSampleCrossingPolicyWorkspaceCompletion(t *testing.T) {
	server := newTestServer(t)
	endActivity := server.gatewayActivity.beginPolicyWorkspace()
	calls := 0
	server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) {
		calls++
		if calls == 1 {
			endActivity()
			return gateway.Status{Gateway: "degraded", DHCP: "stopped"}, nil
		}
		return healthyGatewayStatus(), nil
	}
	cfg, err := config.LoadRuntime(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, view := server.observeGateway(t.Context(), cfg)
	if calls != 2 || view.State != "running" || view.Busy {
		t.Fatalf("calls=%d view=%+v", calls, view)
	}
}
