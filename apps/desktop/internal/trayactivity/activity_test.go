package trayactivity

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func observation(at time.Time) []byte {
	data, _ := json.Marshal(map[string]any{"schema_version": 1, "sampled_at": at, "gateway_rates": map[string]int{"upload": 125, "download": 4000}, "devices": []any{}})
	return data
}

func TestHistorySurvivesWithoutRendererReadsAndRemainsBounded(t *testing.T) {
	now := time.Now().UTC()
	reads := 0
	m := New(func(context.Context) ([]byte, error) { reads++; return observation(now), nil })
	m.now = func() time.Time { return now }
	m.refresh(context.Background())
	if reads != 0 {
		t.Fatal("stopped gateways must not be sampled")
	}
	m.SetRunning(true)
	// No renderer reads occur for two minutes. The native loop still retains
	// exactly the latest minute; reopening needs no new baseline from the UI.
	for range 60 {
		m.refresh(context.Background())
		now = now.Add(2 * time.Second)
	}
	snapshot := m.Snapshot()
	if snapshot.Failed || len(snapshot.History) != 30 || len(snapshot.Traffic) == 0 || snapshot.History[0].Time != now.Add(-time.Minute).UnixMilli() {
		t.Fatalf("unexpected history: %+v", snapshot)
	}
	last := snapshot.History[29]
	if last.Download != 4000 || last.Upload != 125 {
		t.Fatal("native cache must preserve API rates")
	}
	snapshot.History[0].Download = -1
	snapshot.Traffic[0] = '!'
	if next := m.Snapshot(); next.History[0].Download < 0 || !json.Valid(next.Traffic) {
		t.Fatal("callers must not mutate the retained snapshot")
	}
	m.SetRunning(false)
	m.refresh(context.Background())
	if snapshot := m.Snapshot(); reads != 60 || len(snapshot.Traffic) != 0 || len(snapshot.History) != 0 {
		t.Fatal("stop must clear observations and cease reads")
	}
}

func TestGapsDuplicateSamplesAndFailures(t *testing.T) {
	now := time.Now().UTC()
	var readErr error
	m := New(func(context.Context) ([]byte, error) { return observation(now), readErr })
	m.now = func() time.Time { return now }
	m.SetRunning(true)
	m.refresh(context.Background())
	m.refresh(context.Background())
	if len(m.Snapshot().History) != 1 {
		t.Fatal("cached API timestamps must not become duplicate chart points")
	}
	now = now.Add(2 * time.Second)
	m.refresh(context.Background())
	if len(m.Snapshot().History) != 2 {
		t.Fatal("second observation should make rates ready")
	}
	now = now.Add(20 * time.Second)
	if snapshot := m.Snapshot(); !snapshot.Failed || len(snapshot.Traffic) != 0 {
		t.Fatal("stale observations after sleep cannot look live")
	}
	m.refresh(context.Background())
	if len(m.Snapshot().History) != 1 {
		t.Fatal("sleep gap requires a fresh baseline")
	}
	readErr = errors.New("offline")
	m.refresh(context.Background())
	if snapshot := m.Snapshot(); !snapshot.Failed || len(snapshot.History) != 0 || len(snapshot.Traffic) != 0 {
		t.Fatal("a failed read must not appear as zero or healthy traffic")
	}
	readErr = nil
	now = now.Add(2 * time.Second)
	m.refresh(context.Background())
	if snapshot := m.Snapshot(); snapshot.Failed || len(snapshot.History) != 1 {
		t.Fatal("recovery needs a fresh baseline")
	}
}

func TestStopCancelsReadAndRejectsLateResult(t *testing.T) {
	started, done := make(chan struct{}), make(chan struct{})
	m := New(func(ctx context.Context) ([]byte, error) {
		close(started)
		<-ctx.Done()
		return observation(time.Now().UTC()), nil // Simulate a client returning late.
	})
	m.SetRunning(true)
	go func() { m.refresh(context.Background()); close(done) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("read did not start")
	}
	m.SetRunning(false)
	m.SetRunning(true)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel pending observation")
	}
	if snapshot := m.Snapshot(); len(snapshot.Traffic) != 0 || len(snapshot.History) != 0 {
		t.Fatal("old generation overwrote a restarted gateway")
	}
}

func TestRunWakesForGatewayAndCancelsWithHost(t *testing.T) {
	read := make(chan struct{}, 4)
	m := New(func(context.Context) ([]byte, error) { read <- struct{}{}; return observation(time.Now().UTC()), nil })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	m.SetRunning(true)
	select {
	case <-read:
	case <-time.After(time.Second):
		t.Fatal("gateway start did not wake the native observation loop")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("host exit did not stop the observation loop")
	}
}
