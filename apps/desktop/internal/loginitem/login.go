// Package loginitem reports the OS result of explicit login-item changes.
package loginitem

import "sync"

type Snapshot struct {
	State    string `json:"state"`
	Sequence uint64 `json:"sequence"`
	Failed   bool   `json:"failed"`
}

type Provider interface {
	Status() string
	SetEnabled(bool) error
}

type Manager struct {
	mu       sync.Mutex
	provider Provider
	snapshot Snapshot
}

func New(provider Provider) *Manager { return &Manager{provider: provider} }
func (m *Manager) read() Snapshot {
	state := "unavailable"
	if m.provider != nil {
		state = m.provider.Status()
	}
	switch state {
	case "enabled", "disabled", "approval", "unavailable":
	default:
		state = "unavailable"
	}
	m.snapshot.State = state
	m.snapshot.Sequence++
	return m.snapshot
}
func (m *Manager) Snapshot() Snapshot { m.mu.Lock(); defer m.mu.Unlock(); return m.read() }
func (m *Manager) SetEnabled(enabled bool) Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.read()
	m.snapshot.Failed = false
	if m.provider == nil || current.State == "unavailable" {
		m.snapshot.Failed = true
		return m.read()
	}
	if enabled && (current.State == "enabled" || current.State == "approval") || !enabled && current.State == "disabled" {
		return m.read()
	}
	m.snapshot.Failed = m.provider.SetEnabled(enabled) != nil
	return m.read() // Never report the requested value as the actual OS result.
}
