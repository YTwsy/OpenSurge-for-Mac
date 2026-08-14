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
	return m.releaseOwnedLocked(ctx)
}

func (m *systemSleepLeaseManager) releaseOwnedLocked(ctx context.Context) error {
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
	// A marker with no live holders means a previous disconnect could not
	// restore sleep. Retry that cleanup before deciding whether SleepDisabled is
	// externally owned, otherwise the failed release can never recover without a
	// Helper restart.
	if err := m.releaseOwnedLocked(ctx); err != nil {
		return fmt.Errorf("finish pending OpenSurge sleep prevention release: %w", err)
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
	return m.releaseOwnedLocked(ctx)
}

func (m *systemSleepLeaseManager) retryRelease(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			m.mu.Lock()
			if m.holders > 0 {
				m.mu.Unlock()
				cancel()
				return
			}
			err := m.releaseOwnedLocked(attemptCtx)
			m.mu.Unlock()
			cancel()
			if err == nil {
				return
			}
		}
	}
}
