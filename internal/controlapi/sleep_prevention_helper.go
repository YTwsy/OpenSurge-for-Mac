package controlapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var errSleepPreventionExternallyOwned = errors.New("system sleep is already disabled outside OpenSurge")

type systemSleepLeaseManager struct {
	mu      sync.Mutex
	marker  string
	holders int
	inspect func(context.Context) (bool, error)
	set     func(context.Context, bool) error
}

func newSystemSleepLeaseManager(marker string) *systemSleepLeaseManager {
	return &systemSleepLeaseManager{marker: marker, inspect: systemSleepDisabled, set: setSystemSleepDisabled}
}

func (m *systemSleepLeaseManager) Reconcile(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := os.Stat(m.marker); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := m.set(ctx, false); err != nil {
		return fmt.Errorf("release stale OpenSurge sleep prevention: %w", err)
	}
	return os.Remove(m.marker)
}

func (m *systemSleepLeaseManager) Acquire(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.holders > 0 {
		m.holders++
		return nil
	}
	disabled, err := m.inspect(ctx)
	if err != nil {
		return err
	}
	if disabled {
		return errSleepPreventionExternallyOwned
	}
	if err := os.MkdirAll(filepath.Dir(m.marker), 0o755); err != nil {
		return err
	}
	marker, err := os.OpenFile(m.marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create sleep prevention ownership marker: %w", err)
	}
	if closeErr := marker.Close(); closeErr != nil {
		_ = os.Remove(m.marker)
		return closeErr
	}
	if err := m.set(ctx, true); err != nil {
		_ = os.Remove(m.marker)
		return err
	}
	m.holders = 1
	return nil
}

func (m *systemSleepLeaseManager) Release() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.holders == 0 {
		return nil
	}
	m.holders--
	if m.holders > 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.set(ctx, false); err != nil {
		m.holders = 1
		return err
	}
	if err := os.Remove(m.marker); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
