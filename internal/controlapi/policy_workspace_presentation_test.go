package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/gatewayview"
	"open-mihomo-gateway/internal/runtime"
)

func TestPolicyWorkspacePreservesGatewayPresentation(t *testing.T) {
	const probe = `{"action":"test","names":["NodeA"]}`
	for _, test := range []struct {
		name, body, state, reason string
		stream, fail, cancel      bool
	}{
		{name: "read", body: `{"action":"read"}`, state: "running"},
		{name: "select", body: `{"action":"select","group":"Main","policy":"DIRECT"}`, state: "running"},
		{name: "probe JSON", body: probe, state: "running"},
		{name: "probe stream", body: probe, state: "running", stream: true},
		{name: "stopped gateway", body: probe, state: "stopped", stream: true},
		{name: "component failure", body: probe, state: "degraded", reason: "dns_stopped", stream: true},
		{name: "probe error", body: probe, state: "running", fail: true},
		{name: "stream error", body: probe, state: "running", stream: true, fail: true},
		{name: "stream cancellation", body: probe, state: "running", stream: true, cancel: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newTestServer(t)
			t.Cleanup(func() { _ = server.policyWorkspaceLease.Close() })
			status := healthyGatewayStatus()
			status.Gateway = test.state
			if test.reason == "dns_stopped" {
				status.DHCP = "stopped"
			}
			server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) { return status, nil }
			cfg, err := config.LoadRuntime(server.configPath)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			called := false
			server.policyWorkspaceRunner = streamingWorkspaceRunner{run: func(ctx context.Context) (PolicyWorkspaceResponse, error) {
				called = true
				// Reproduce the Helper's real cross-process lock while the request
				// is still active, without changing any host networking.
				err := runtime.WithLifecycleLock(cfg, func() error {
					assertWorkspaceGatewayPresentation(t, server, test.state, test.reason, true)
					response := performAuthorized(server, http.MethodPost, "/api/v1/gateway/stop", nil)
					if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "operation_in_progress") {
						t.Fatalf("workspace lost lifecycle exclusion: %d %s", response.Code, response.Body.String())
					}
					if test.cancel {
						cancel()
						return ctx.Err()
					}
					if test.fail {
						return errors.New("node probe failed")
					}
					return nil
				})
				return PolicyWorkspaceResponse{}, err
			}}
			request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:61767/api/v1/policy-workspace", strings.NewReader(test.body)).WithContext(ctx)
			request.Header.Set("Authorization", "Bearer "+server.token)
			if test.stream {
				request.Header.Set("Accept", "text/event-stream")
			}
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			wantCode := http.StatusOK
			if test.fail && !test.stream {
				wantCode = http.StatusBadGateway
			}
			if !called || response.Code != wantCode {
				t.Fatalf("workspace response: called=%t status=%d body=%s", called, response.Code, response.Body.String())
			}
			assertWorkspaceGatewayPresentation(t, server, test.state, test.reason, false)
			// Completion, failure and cancellation must all release ownership;
			// a subsequent external lifecycle action must still be visible.
			if err := runtime.WithLifecycleLock(cfg, func() error {
				assertWorkspaceGatewayPresentation(t, server, "changing", "external_operation", true)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func assertWorkspaceGatewayPresentation(t *testing.T, server *Server, state, reason string, busy bool) {
	t.Helper()
	presentations := make(map[string]gatewayview.Status)
	for _, path := range []string{"/api/v1/overview", "/api/v1/menubar"} {
		response := performAuthorized(server, http.MethodGet, path, nil)
		var data MenuBarStatus
		if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil || response.Code != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s err=%v", path, response.Code, response.Body.String(), err)
		}
		presentations[path] = data.Presentation
	}
	event, err := server.stateEvent(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	presentations["state event"] = event.Presentation
	for path, view := range presentations {
		if view.State != state || view.Reason != reason || view.Busy != busy || view.OperationID != "" || view.Phase != "" {
			t.Errorf("%s: presentation=%+v, want state=%s reason=%s busy=%t without lifecycle progress", path, view, state, reason, busy)
		}
	}
}
