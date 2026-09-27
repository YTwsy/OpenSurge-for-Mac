// Package menustatus adapts the existing Control API DTO for the desktop host.
// It contains display/exit eligibility rules, never gateway operations.
package menustatus

import (
	"context"
	"strings"
	"sync"
	"time"
)

type SleepPrevention struct {
	Enabled bool   `json:"enabled"`
	Active  bool   `json:"active"`
	Error   string `json:"error,omitempty"`
}

type Status struct {
	SchemaVersion    int             `json:"schema_version"`
	Revision         string          `json:"revision"`
	Gateway          string          `json:"gateway"`
	Topology         string          `json:"topology"`
	LANIP            string          `json:"lan_ip"`
	DHCP             string          `json:"dhcp"`
	Mihomo           string          `json:"mihomo"`
	TUN              string          `json:"tun"`
	TUNInterface     string          `json:"tun_interface,omitempty"`
	PFAnchor         string          `json:"pf_anchor"`
	Forwarding       string          `json:"forwarding"`
	IPv4Takeover     string          `json:"ipv4_takeover"`
	IPv6Takeover     string          `json:"ipv6_takeover"`
	ClientCount      int             `json:"client_count"`
	Drift            bool            `json:"drift"`
	DoctorHealthy    bool            `json:"doctor_healthy"`
	RecoveryRequired bool            `json:"recovery_required"`
	RecoveryStage    string          `json:"recovery_stage,omitempty"`
	ErrorCode        string          `json:"error_code,omitempty"`
	Warnings         []string        `json:"warnings"`
	SleepPrevention  SleepPrevention `json:"sleep_prevention"`
	UIPreferences    struct {
		Language string `json:"language"`
	} `json:"ui_preferences"`
}

func (s Status) RecoveryNeedsAttention() bool {
	if !s.RecoveryRequired {
		return false
	}
	switch s.RecoveryStage {
	case "prepared", "gateway_active", "client_validated", "client_validation_skipped":
		return false
	default:
		return true
	}
}
func (s Status) ServicesActive() bool {
	return s.Gateway == "running" || s.Gateway == "degraded" || s.DHCP == "running" || strings.HasPrefix(s.Mihomo, "running") || s.PFAnchor == "loaded"
}
func (s Status) CanQuit() bool {
	return s.Gateway == "stopped" && !s.ServicesActive() && !s.RecoveryNeedsAttention()
}
func (s Status) Indicator() string {
	if s.RecoveryNeedsAttention() {
		return "recovery"
	}
	if s.Gateway == "stopped" {
		return "stopped"
	}
	if s.Gateway == "degraded" || s.Drift || !s.DoctorHealthy {
		return "degraded"
	}
	if s.Gateway == "running" {
		return "running"
	}
	return "unreachable"
}

type Snapshot struct {
	Status    *Status `json:"status"`
	Indicator string  `json:"indicator"`
	Sequence  uint64  `json:"sequence"`
	CanQuit   bool    `json:"can_quit"`
}

type Reader func(context.Context) (*Status, error)

type Monitor struct {
	read Reader
	// op serialises native status reads with acknowledged sleep changes. A read
	// started before a mutation cannot overwrite its authoritative response.
	op       sync.Mutex
	mu       sync.Mutex
	snapshot Snapshot
	failures int
	rapid    bool
	wake     chan struct{}
	changed  func(Snapshot)
}

func New(read Reader, changed func(Snapshot)) *Monitor {
	return &Monitor{read: read, changed: changed, snapshot: Snapshot{Indicator: "connecting"}, wake: make(chan struct{}, 1)}
}
func (m *Monitor) Snapshot() Snapshot { m.mu.Lock(); defer m.mu.Unlock(); return m.snapshot }
func (m *Monitor) Refresh(ctx context.Context) Snapshot {
	m.op.Lock()
	defer m.op.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	status, err := m.read(ctx)
	m.mu.Lock()
	m.snapshot.Sequence++
	m.snapshot.Status = nil
	m.snapshot.CanQuit = false
	if err != nil || status == nil || status.SchemaVersion != 1 || status.Gateway == "" {
		m.failures++
		m.snapshot.Indicator = "unreachable"
	} else {
		m.failures = 0
		m.snapshot.Status = status
		m.snapshot.Indicator = status.Indicator()
		m.snapshot.CanQuit = status.CanQuit()
	}
	result := m.snapshot
	m.mu.Unlock()
	if m.changed != nil {
		m.changed(result)
	}
	return result
}
func (m *Monitor) SetSleep(ctx context.Context, write func(context.Context) (SleepPrevention, error)) (SleepPrevention, error) {
	m.op.Lock()
	defer m.op.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	result, err := write(ctx)
	if err != nil {
		return result, err
	}
	m.mu.Lock()
	if m.snapshot.Status != nil {
		status := *m.snapshot.Status
		status.SleepPrevention = result
		m.snapshot.Status = &status
	}
	m.snapshot.Sequence++
	snapshot := m.snapshot
	m.mu.Unlock()
	if m.changed != nil {
		m.changed(snapshot)
	}
	return result, nil
}
func (m *Monitor) SetRapid(rapid bool) {
	m.mu.Lock()
	m.rapid = rapid
	m.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *Monitor) interval() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	interval := 15 * time.Second
	if m.rapid {
		interval = 2 * time.Second
	}
	for i := 0; i < min(m.failures, 4); i++ {
		interval *= 2
	}
	return min(interval, time.Minute)
}
func (m *Monitor) Run(ctx context.Context) {
	for ctx.Err() == nil {
		m.Refresh(ctx)
		timer := time.NewTimer(m.interval())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-m.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}
