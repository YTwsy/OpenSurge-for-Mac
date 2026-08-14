package controlapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/doctor"
	"open-mihomo-gateway/internal/gateway"
)

func TestDoctorControllerRunsSingleFlightAndMarksOldResultsStale(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	runs := 0
	controller := newDoctorController(func(config.Config) doctor.Report {
		mu.Lock()
		runs++
		mu.Unlock()
		close(started)
		<-release
		return doctor.Report{Checks: []doctor.Check{
			{Name: "root privileges", OK: false},
			{Name: "mihomo config validation", OK: false, Message: "invalid"},
		}}
	})

	status, accepted := controller.start(config.Config{}, "revision-a")
	if !accepted || status.State != doctorRunRunning || !status.Current {
		t.Fatalf("first start = %#v, accepted=%v", status, accepted)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Doctor runner did not start")
	}
	status, accepted = controller.start(config.Config{}, "revision-b")
	if accepted || status.State != doctorRunRunning || status.Current {
		t.Fatalf("concurrent start = %#v, accepted=%v", status, accepted)
	}
	mu.Lock()
	gotRuns := runs
	mu.Unlock()
	if gotRuns != 1 {
		t.Fatalf("Doctor runs=%d, want one", gotRuns)
	}

	close(release)
	waitForDoctorState(t, controller, doctorRunSucceeded)
	status = controller.snapshot("revision-a")
	if !status.Current || status.Healthy || len(status.Checks) != 1 || status.Checks[0].Name != "mihomo config validation" {
		t.Fatalf("completed status = %#v", status)
	}
	if current := controller.snapshot("revision-b"); current.Current {
		t.Fatalf("old result was not marked stale: %#v", current)
	}
}

func TestDoctorControllerReportsRunnerPanicInsteadOfStayingRunning(t *testing.T) {
	controller := newDoctorController(func(config.Config) doctor.Report {
		panic("boom")
	})
	controller.start(config.Config{}, "revision")
	waitForDoctorState(t, controller, doctorRunFailed)
	status := controller.snapshot("revision")
	if status.Error != "Doctor panicked: boom" || status.Healthy {
		t.Fatalf("panic status = %#v", status)
	}
}

func TestOverviewAndMenuBarOnlyReadExplicitDoctorResult(t *testing.T) {
	server := newTestServer(t)
	server.gatewayStatus = func(context.Context, config.Config) (gateway.Status, error) {
		return gateway.Status{Gateway: "running", RuntimeState: "active", Mihomo: "running"}, nil
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var mu sync.Mutex
	runs := 0
	server.doctor = newDoctorController(func(config.Config) doctor.Report {
		mu.Lock()
		runs++
		mu.Unlock()
		close(started)
		<-release
		return doctor.Report{Checks: []doctor.Check{{Name: "mihomo config validation", OK: false, Message: "invalid"}}}
	})

	for _, path := range []string{"/api/v1/overview", "/api/v1/menubar"} {
		response := performAuthorized(server, http.MethodGet, path, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	mu.Lock()
	gotRuns := runs
	mu.Unlock()
	if gotRuns != 0 {
		t.Fatalf("hot-path requests ran Doctor %d times", gotRuns)
	}

	response := performAuthorized(server, http.MethodPost, "/api/v1/doctor", nil)
	if response.Code != http.StatusAccepted {
		t.Fatalf("POST Doctor status=%d body=%s", response.Code, response.Body.String())
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("explicit Doctor request did not start the runner")
	}
	response = performAuthorized(server, http.MethodPost, "/api/v1/doctor", nil)
	if response.Code != http.StatusAccepted {
		t.Fatalf("second POST Doctor status=%d body=%s", response.Code, response.Body.String())
	}
	mu.Lock()
	gotRuns = runs
	mu.Unlock()
	if gotRuns != 1 {
		t.Fatalf("concurrent explicit requests ran Doctor %d times", gotRuns)
	}

	releaseOnce.Do(func() { close(release) })
	waitForDoctorState(t, server.doctor, doctorRunSucceeded)
	response = performAuthorized(server, http.MethodGet, "/api/v1/overview", nil)
	var overview Overview
	if err := json.Unmarshal(response.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if overview.DoctorHealthy || len(overview.Doctor) != 1 || overview.DoctorStatus.State != doctorRunSucceeded || !overview.DoctorStatus.Current {
		t.Fatalf("overview cached Doctor = %#v", overview)
	}

	data, err := os.ReadFile(server.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(server.configPath, append(data, []byte("\n# revision changed\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	response = performAuthorized(server, http.MethodGet, "/api/v1/overview", nil)
	if err := json.Unmarshal(response.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if !overview.DoctorHealthy || len(overview.Doctor) != 0 || overview.DoctorStatus.Current {
		t.Fatalf("stale Doctor affected current overview = %#v", overview)
	}
}

func waitForDoctorState(t *testing.T, controller *doctorController, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if controller.snapshot("").State == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("Doctor state=%#v, want %q", controller.snapshot(""), want)
}
