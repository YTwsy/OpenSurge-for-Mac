package menustatus

import (
	"context"
	"errors"
	"testing"
	"time"

	"open-mihomo-gateway/internal/gatewayview"
)

func TestExitAndIndicatorMatchExistingHostContract(t *testing.T) {
	stopped := Status{SchemaVersion: 1, Gateway: "stopped", DHCP: "stopped", Mihomo: "stopped", PFAnchor: "unloaded", Forwarding: "enabled", Drift: true}
	if !stopped.CanQuit() || stopped.Indicator() != "stopped" {
		t.Fatal("stopped runtime checks or unrelated forwarding blocked exit")
	}
	for _, service := range []string{"dhcp", "mihomo", "pf"} {
		active := stopped
		switch service {
		case "dhcp":
			active.DHCP = "running"
		case "mihomo":
			active.Mihomo = "running"
		case "pf":
			active.PFAnchor = "loaded"
		}
		if active.CanQuit() {
			t.Fatalf("active %s permitted full quit", service)
		}
	}
	for _, stage := range []string{"", "mac_static", "gateway_stopped_waiting_router_dhcp", "router_dhcp_restored"} {
		recovery := stopped
		recovery.RecoveryRequired = true
		recovery.RecoveryStage = stage
		if recovery.CanQuit() || recovery.Indicator() != "recovery" {
			t.Fatalf("recovery %q lost priority", stage)
		}
	}
	stopped.RecoveryRequired = true
	stopped.RecoveryStage = "prepared"
	if !stopped.CanQuit() {
		t.Fatal("prepared snapshot blocks exit")
	}
	stopped.Mihomo = "running (1.19.30-opensurge.1)"
	if stopped.CanQuit() {
		t.Fatal("versioned running engine permitted exit")
	}
}

func TestPresentationKeepsPendingConfigHealthyAndTransitionsNonActionable(t *testing.T) {
	status := Status{SchemaVersion: 1, Gateway: "running", DHCP: "running", Mihomo: "running", Drift: true, DoctorHealthy: false}
	if status.Indicator() != "running" {
		t.Fatal("legacy pending configuration/Doctor became a runtime fault")
	}
	status.Presentation = gatewayview.Status{State: "running", ConfigPending: true, DiagnosisWarning: true}
	if status.Indicator() != "running" {
		t.Fatal("pending configuration/Doctor became a runtime fault")
	}
	for _, state := range []string{"starting", "reloading", "stopping", "recovering", "rolling_back", "changing", "unknown"} {
		status.Gateway, status.DHCP, status.Mihomo, status.PFAnchor = "stopped", "stopped", "stopped", "unloaded"
		status.Presentation = gatewayview.Status{State: state, Busy: true}
		if status.Indicator() != state || status.CanQuit() {
			t.Fatalf("%s: %+v", state, status)
		}
	}
	monitor := New(func(context.Context) (*Status, error) { return &status, nil }, nil)
	monitor.Refresh(context.Background())
	if monitor.interval() != time.Second {
		t.Fatal("in-flight operation retained idle polling interval")
	}
	status.Gateway = ""
	status.Presentation = gatewayview.Status{State: "unknown", Reason: "status_unavailable"}
	if snapshot := monitor.Refresh(context.Background()); snapshot.Indicator != "unknown" || snapshot.CanQuit {
		t.Fatalf("unknown snapshot = %+v", snapshot)
	}
}

func TestFailureClearsHealthyStateAndAcknowledgedSleepSurvivesOldSnapshot(t *testing.T) {
	status := &Status{SchemaVersion: 1, Gateway: "running", DoctorHealthy: true}
	fail := false
	monitor := New(func(context.Context) (*Status, error) {
		if fail {
			return nil, errors.New("offline")
		}
		copy := *status
		return &copy, nil
	}, nil)
	initial := monitor.Refresh(context.Background())
	_, err := monitor.SetSleep(context.Background(), func(context.Context) (SleepPrevention, error) {
		return SleepPrevention{Enabled: true, Active: true}, nil
	})
	if err != nil || !monitor.Snapshot().Status.SleepPrevention.Active || initial.Status.SleepPrevention.Active {
		t.Fatal("acknowledgement or immutable snapshot lost")
	}
	fail = true
	offline := monitor.Refresh(context.Background())
	if offline.Status != nil || offline.Indicator != "unreachable" || offline.CanQuit {
		t.Fatal("stale healthy state remained actionable")
	}
	monitor.SetRapid(true)
	if monitor.interval().Seconds() != 4 {
		t.Fatal("visible failure backoff")
	}
}

func TestOlderRefreshCannotOverwriteConcurrentSleepAcknowledgement(t *testing.T) {
	started, finishRead, readDone, writeDone := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	monitor := New(func(context.Context) (*Status, error) {
		close(started)
		<-finishRead
		return &Status{SchemaVersion: 1, Gateway: "stopped"}, nil
	}, nil)
	go func() { monitor.Refresh(context.Background()); close(readDone) }()
	<-started
	go func() {
		_, err := monitor.SetSleep(context.Background(), func(context.Context) (SleepPrevention, error) {
			return SleepPrevention{Active: true, Enabled: true}, nil
		})
		if err != nil {
			t.Error(err)
		}
		close(writeDone)
	}()
	close(finishRead)
	<-readDone
	<-writeDone
	if !monitor.Snapshot().Status.SleepPrevention.Active {
		t.Fatal("old read overwrote acknowledged sleep change")
	}
}
