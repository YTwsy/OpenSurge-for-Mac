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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"open-mihomo-gateway/internal/gateway"
)

func TestOperationProgressPersistsPhaseAndNoticesUntilCompletion(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	server := &Server{store: store}
	op := newOperation("progress-test", "reload")
	if err := store.CreateOperation(op); err != nil {
		t.Fatal(err)
	}
	ctx := server.observeOperation(context.Background(), &op)
	gateway.ReportProgress(ctx, "validating_config")
	first, err := store.Operation(op.ID)
	if err != nil || first.State != "running" || first.Phase != "validating_config" || first.PhaseStartedAt.IsZero() {
		t.Fatalf("running progress = %+v, %v", first, err)
	}
	gateway.ReportProgress(ctx, "validating_config")
	if !op.PhaseStartedAt.Equal(first.PhaseStartedAt) {
		t.Fatal("unchanged phase reset its start time")
	}
	report := gateway.ProgressReporter(ctx)
	report(gateway.Progress{Notice: "tailscale_warmup_started"})
	report(gateway.Progress{Notice: "tailscale_warmup_started"})
	gateway.ReportProgress(ctx, "rolling_back")
	server.finishOperation(&op, errors.New("test startup failed"))
	gateway.ReportProgress(ctx, "starting_mihomo") // no late mutation of terminal history
	final, err := store.Operation(op.ID)
	if err != nil || final.State != "failed" || final.Phase != "rolling_back" || final.Error != "test startup failed" || !slices.Equal(final.Notices, []string{"tailscale_warmup_started"}) {
		t.Fatalf("final progress = %+v, %v", final, err)
	}
}

func TestFirstGatewayStartExposesProgressBeforeCompleting(t *testing.T) {
	server := newTestServer(t)
	if err := server.store.SaveRecovery(RecoveryState{Stage: RecoveryRouterDHCPDisabledConfirmed, Required: true}); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	server.runner = actionRunnerFunc(func(ctx context.Context, action, _ string) error {
		if action != "start" {
			return errors.New("unexpected action")
		}
		gateway.ReportProgress(ctx, "starting_mihomo")
		close(entered)
		<-release
		gateway.ProgressReporter(ctx)(gateway.Progress{Notice: "tailscale_warmup_started"})
		return nil
	})
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:61767/api/v1/gateway/start", nil)
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("Idempotency-Key", "first-start")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", response.Code, response.Body.String())
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}
	progress := performAuthorized(server, http.MethodGet, "/api/v1/operations/first-start", nil)
	if progress.Code != http.StatusOK || !containsAll(progress.Body.String(), `"phase":"starting_mihomo"`, `"state":"running"`, `"phase_started_at":`) {
		t.Fatalf("in-flight progress: %d %s", progress.Code, progress.Body.String())
	}
	// An in-flight replay is still idempotent, never a second start.
	repeated := httptest.NewRecorder()
	server.Handler().ServeHTTP(repeated, request)
	if repeated.Code != http.StatusAccepted {
		t.Fatalf("repeated start: %d", repeated.Code)
	}
	unblock()
	op := waitForStoredOperation(t, server, "first-start", "succeeded")
	if !slices.Contains(op.Notices, "tailscale_warmup_started") {
		t.Fatal("warm-up notice lost")
	}
	recovery, _ := server.store.Recovery()
	if recovery.Stage != RecoveryGatewayActive {
		t.Fatalf("start skipped client acceptance: %s", recovery.Stage)
	}
}

type progressDevicePolicyRunner struct {
	fakeConfigurationRunner
	entered chan struct{}
	release chan struct{}
}

func (runner progressDevicePolicyRunner) ApplyDevicePolicy(ctx context.Context, path, revision string, payload []byte) (string, error) {
	gateway.ReportProgress(ctx, "validating_config")
	close(runner.entered)
	<-runner.release
	gateway.ReportProgress(ctx, "saving_config")
	return runner.fakeConfigurationRunner.ApplyDevicePolicy(ctx, path, revision, payload)
}

func TestDevicePolicySaveProgressIsReadableDuringSynchronousRequest(t *testing.T) {
	server := newTestServer(t)
	current := performAuthorized(server, http.MethodGet, "/api/v1/device-policy", nil)
	var document DevicePolicyResponse
	if err := json.Unmarshal(current.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(document.Policy)
	runner := progressDevicePolicyRunner{entered: make(chan struct{}), release: make(chan struct{})}
	server.configRunner = runner
	var once sync.Once
	unblock := func() { once.Do(func() { close(runner.release) }) }
	defer unblock()
	request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:61767/api/v1/device-policy", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("If-Match", `"`+document.Revision+`"`)
	request.Header.Set(operationIDHeader, "device-save")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { server.Handler().ServeHTTP(response, request); close(done) }()
	select {
	case <-runner.entered:
	case <-time.After(time.Second):
		t.Fatal("save runner did not start")
	}
	progress := performAuthorized(server, http.MethodGet, "/api/v1/operations/device-save", nil)
	if !containsAll(progress.Body.String(), `"kind":"save-device-policy"`, `"phase":"validating_config"`, `"state":"running"`) {
		t.Fatalf("save progress: %s", progress.Body.String())
	}
	unblock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("save request did not finish")
	}
	if response.Code != http.StatusOK || response.Header().Get(operationIDHeader) != "device-save" {
		t.Fatalf("save response: %d %s", response.Code, response.Body.String())
	}
	op := waitForStoredOperation(t, server, "device-save", "succeeded")
	if op.Phase != "saving_config" {
		t.Fatalf("final phase = %s", op.Phase)
	}
}

func TestOperationIDsCannotOverwriteOtherFilesOrExistingRequests(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "../config", "a/b", ".", "..", "a\\b", strings.Repeat("x", 129)} {
		if err := store.SaveOperation(newOperation(id, "reload")); err == nil {
			t.Fatalf("unsafe operation ID accepted: %q", id)
		}
	}
	op := newOperation("one-request", "reload")
	if err := store.CreateOperation(op); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateOperation(newOperation(op.ID, "stop")); !errors.Is(err, errOperationExists) {
		t.Fatalf("duplicate operation: %v", err)
	}
	stored, err := store.Operation(op.ID)
	if err != nil || stored.Kind != "reload" {
		t.Fatalf("existing operation changed: %+v, %v", stored, err)
	}
}

func TestHelperClientAcceptsProgressFramesAndLegacyFinalOnlyResponses(t *testing.T) {
	for _, streaming := range []bool{true, false} {
		t.Run(map[bool]string{true: "streaming", false: "legacy"}[streaming], func(t *testing.T) {
			// Keep the Unix socket path below macOS's sockaddr_un limit.
			dir, err := os.MkdirTemp("", "os-helper-")
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
			serverDone := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					serverDone <- err
					return
				}
				defer conn.Close()
				var request HelperRequest
				if err := json.NewDecoder(conn).Decode(&request); err != nil {
					serverDone <- err
					return
				}
				if !request.WatchProgress {
					serverDone <- errors.New("client did not opt in to progress")
					return
				}
				encoder := json.NewEncoder(conn)
				if streaming {
					if err := encoder.Encode(HelperResponse{Progress: &gateway.Progress{Phase: "validating_config"}}); err != nil {
						serverDone <- err
						return
					}
					if err := encoder.Encode(HelperResponse{Progress: &gateway.Progress{Notice: "tailscale_warmup_started"}}); err != nil {
						serverDone <- err
						return
					}
				}
				serverDone <- encoder.Encode(HelperResponse{OK: true})
			}()
			var progress []gateway.Progress
			ctx := gateway.WithProgress(context.Background(), func(p gateway.Progress) { progress = append(progress, p) })
			if err := (HelperClient{SocketPath: path}).Run(ctx, "start", "/unused/config.yaml"); err != nil {
				t.Fatal(err)
			}
			if err := <-serverDone; err != nil {
				t.Fatal(err)
			}
			if streaming && !slices.Equal(progress, []gateway.Progress{{Phase: "validating_config"}, {Notice: "tailscale_warmup_started"}}) {
				t.Fatalf("progress frames = %+v", progress)
			}
			if !streaming && len(progress) != 0 {
				t.Fatal("legacy helper fabricated progress")
			}
		})
	}
}

func TestSlowProgressObserverCannotHoldUpLifecycleCleanup(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	ctx := withHelperProgress(context.Background(), server)
	started := time.Now()
	gateway.ReportProgress(ctx, "stopping_mihomo") // client intentionally never reads
	gateway.ReportProgress(ctx, "restoring_network")
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("observer held up cleanup for %s", elapsed)
	}
}
