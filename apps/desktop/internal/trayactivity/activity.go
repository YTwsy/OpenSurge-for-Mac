// Package trayactivity retains a bounded view of Control API observations even
// when both WebViews are hidden. Rate calculation remains in the Control Service.
package trayactivity

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

const maxGap = 15 * time.Second // Same validity window as the Control API sampler.

type Point struct {
	Time     int64 `json:"time"`
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

type Snapshot struct {
	Traffic json.RawMessage `json:"traffic"`
	History []Point         `json:"history"`
	Failed  bool            `json:"failed"`
}

type Monitor struct {
	read       func(context.Context) ([]byte, error)
	now        func() time.Time
	mu         sync.Mutex
	op         sync.Mutex
	running    bool
	generation uint64
	cancel     context.CancelFunc
	snapshot   Snapshot
	sampledAt  time.Time
	failures   int
	wake       chan struct{}
}

func New(read func(context.Context) ([]byte, error)) *Monitor {
	return &Monitor{read: read, now: time.Now, wake: make(chan struct{}, 1)}
}

func (m *Monitor) SetRunning(running bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running == running {
		return
	}
	m.running = running
	m.generation++
	m.snapshot, m.sampledAt, m.failures = Snapshot{}, time.Time{}, 0
	if m.cancel != nil {
		m.cancel()
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Monitor) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.sampledAt.IsZero() && (m.now().Sub(m.sampledAt) > maxGap || m.now().Before(m.sampledAt)) {
		return Snapshot{Failed: true}
	}
	return Snapshot{Traffic: append(json.RawMessage(nil), m.snapshot.Traffic...), History: append([]Point(nil), m.snapshot.History...), Failed: m.snapshot.Failed}
}

func (m *Monitor) refresh(ctx context.Context) {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	if !m.running || ctx.Err() != nil {
		m.mu.Unlock()
		return
	}
	generation := m.generation
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	m.cancel = cancel
	m.mu.Unlock()
	defer cancel()
	data, err := m.read(ctx)
	var sample struct {
		SchemaVersion   int       `json:"schema_version"`
		SampledAt       time.Time `json:"sampled_at"`
		ConnectionError string    `json:"connection_error"`
		Rates           struct {
			Upload   int64 `json:"upload"`
			Download int64 `json:"download"`
		} `json:"gateway_rates"`
	}
	parseErr := json.Unmarshal(data, &sample)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancel = nil
	if generation != m.generation || !m.running || ctx.Err() == context.Canceled {
		return // A stopped/restarted gateway cannot be overwritten by an old read.
	}
	if err != nil || ctx.Err() != nil || parseErr != nil || sample.SchemaVersion != 1 || sample.SampledAt.IsZero() || sample.ConnectionError != "" || m.now().Sub(sample.SampledAt) > maxGap || sample.SampledAt.After(m.now()) {
		m.snapshot, m.sampledAt = Snapshot{Failed: true}, time.Time{}
		m.failures++
		return
	}
	m.failures = 0
	history := m.snapshot.History
	if sample.SampledAt.Before(m.sampledAt) || sample.SampledAt.Sub(m.sampledAt) > maxGap {
		history = nil // Sleep/service gaps need a new baseline, never invented zeroes.
	}
	if !sample.SampledAt.Equal(m.sampledAt) {
		history = append(history, Point{Time: sample.SampledAt.UnixMilli(), Upload: sample.Rates.Upload, Download: sample.Rates.Download})
	}
	cutoff := sample.SampledAt.Add(-time.Minute).UnixMilli()
	for len(history) > 30 || (len(history) > 0 && history[0].Time <= cutoff) {
		history = history[1:]
	}
	m.sampledAt = sample.SampledAt
	m.snapshot = Snapshot{Traffic: append(json.RawMessage(nil), data...), History: history}
}

func (m *Monitor) Run(ctx context.Context) {
	for ctx.Err() == nil {
		m.refresh(ctx)
		m.mu.Lock()
		running, failures := m.running, m.failures
		m.mu.Unlock()
		var tick <-chan time.Time
		var timer *time.Timer
		if running {
			timer = time.NewTimer(min(2*time.Second*time.Duration(1<<min(failures, 4)), 30*time.Second))
			tick = timer.C
		}
		select {
		case <-ctx.Done():
		case <-m.wake:
		case <-tick:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}
