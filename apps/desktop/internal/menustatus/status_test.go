package menustatus

import (
	"context"
	"errors"
	"testing"
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
