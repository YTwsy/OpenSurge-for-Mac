// Package uninstall hands a fixed mode to the existing installed uninstaller.
// The script remains the authority for privileged cleanup and rechecks stopped state.
package uninstall

import (
	"context"
	"errors"
	"sync"

	"open-mihomo-gateway/apps/desktop/internal/loginitem"
	"open-mihomo-gateway/apps/desktop/internal/menustatus"
)

var (
	ErrUnavailable = errors.New("installed uninstaller unavailable")
	ErrUnsafe      = errors.New("gateway must be stopped before uninstall")
	ErrCancelled   = errors.New("uninstall authorization cancelled")
	ErrFailed      = errors.New("uninstall failed")
	ErrLogin       = errors.New("login item could not be disabled")
	ErrRestore     = errors.New("uninstall failed and login item could not be restored")
)

type Mode string

const (
	KeepData  Mode = "keep-data"
	RemoveAll Mode = "remove-all"
)

type Login interface {
	Snapshot() loginitem.Snapshot
	SetEnabled(bool) loginitem.Snapshot
}
type Manager struct {
	mu       sync.Mutex
	state    string
	validate func() error
	read     func(context.Context) (*menustatus.Status, error)
	run      func(Mode) error
	login    Login
	done     bool
}

func New(state string, validate func() error, read func(context.Context) (*menustatus.Status, error), run func(Mode) error, login Login) *Manager {
	return &Manager{state: state, validate: validate, read: read, run: run, login: login}
}
func (m *Manager) Availability() string { return m.state }
func CanUninstall(s *menustatus.Status) bool {
	// Recovery may still need manual steps after uninstall, as in the Swift host.
	// Existing host forwarding alone is not a gateway-owned service.
	return s != nil && s.SchemaVersion == 1 && s.Gateway == "stopped" &&
		(s.DHCP == "stopped" || s.DHCP == "disabled") && s.Mihomo == "stopped" &&
		(s.PFAnchor == "unloaded" || s.PFAnchor == "disabled")
}
func (m *Manager) Check(ctx context.Context) error {
	if m.state != "available" {
		return ErrUnavailable
	}
	if err := m.validate(); err != nil {
		return ErrUnavailable
	}
	status, err := m.read(ctx)
	if err != nil || !CanUninstall(status) {
		return ErrUnsafe
	}
	return nil
}
func (m *Manager) Run(ctx context.Context, mode Mode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done || (mode != KeepData && mode != RemoveAll) {
		return ErrUnavailable
	}
	if err := m.Check(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	login := m.login.Snapshot()
	if login.State == "unavailable" {
		return ErrLogin
	}
	restore := login.State == "enabled" || login.State == "approval"
	if restore {
		result := m.login.SetEnabled(false)
		if result.Failed || result.State != "disabled" {
			return ErrLogin
		}
	}
	// Once authorisation begins, a WebView disconnect must not cancel the script
	// halfway through cleanup. Its native authorisation prompt remains cancellable.
	err := m.run(mode)
	if err != nil {
		if restore {
			result := m.login.SetEnabled(true)
			if result.Failed || (result.State != "enabled" && result.State != "approval") {
				return ErrRestore
			}
		}
		return err
	}
	m.done = true
	return nil
}
