package gateway

import (
	"context"
	"fmt"
	"path/filepath"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

var ErrLifecycleOperationInProgress = runtime.ErrLifecycleOperationInProgress

type lifecycleLock struct {
	lock *runtime.LifecycleLock
}

func acquireLifecycleLock(cfg config.Config) (*lifecycleLock, error) {
	lock, err := runtime.AcquireLifecycleLock(cfg)
	if err != nil {
		return nil, err
	}
	return &lifecycleLock{lock: lock}, nil
}

func (l *lifecycleLock) release() error {
	if l == nil {
		return nil
	}
	return l.lock.Release()
}

// LifecycleOperationInProgress lets the unprivileged control service avoid
// mistaking an intentional stop/reload window from the CLI or privileged
// helper for a crashed mihomo process. The lock file contains no state; the
// kernel-held advisory lock is the only authority.
func LifecycleOperationInProgress(cfg config.Config) (bool, error) {
	return runtime.LifecycleOperationInProgress(cfg)
}

func (m Manager) withLifecycleLock(run func() error) error {
	return runtime.WithLifecycleLock(m.cfg, run)
}

type configLifecycleDeps struct {
	loadRuntime func(string) (config.Config, error)
	loadLocked  func(string) (config.Config, error)
	runLocked   func(context.Context, config.Config) error
}

// StartConfig acquires the runtime lifecycle lock before loading the desired
// configuration used for startup. This closes the read-before-lock race where
// another privileged transaction could persist a new graph after a CLI had
// already captured the old one in memory.
func StartConfig(ctx context.Context, configPath string) error {
	return runConfigLifecycle(ctx, configPath, configLifecycleDeps{
		loadRuntime: config.LoadRuntime,
		loadLocked:  config.Load,
		runLocked: func(ctx context.Context, cfg config.Config) error {
			manager := New(cfg)
			return manager.StartLocked(ctx)
		},
	})
}

// ReloadConfig follows the same lock-before-load rule as StartConfig so a
// delayed CLI reload cannot replace a newly applied live graph with the old
// configuration it observed before the apply transaction.
func ReloadConfig(ctx context.Context, configPath string) error {
	return runConfigLifecycle(ctx, configPath, configLifecycleDeps{
		loadRuntime: config.LoadRuntime,
		loadLocked:  config.Load,
		runLocked: func(ctx context.Context, cfg config.Config) error {
			manager := New(cfg)
			return manager.ReloadLocked(ctx)
		},
	})
}

// RestartMihomoConfig reloads runtime-safe desired fields only after taking
// the lifecycle lock. It deliberately continues to defer a mutable invalid
// device-policy draft so recovery of the already-applied gateway stays usable.
func RestartMihomoConfig(ctx context.Context, configPath string) error {
	return runConfigLifecycle(ctx, configPath, configLifecycleDeps{
		loadRuntime: config.LoadRuntime,
		loadLocked:  config.LoadRuntime,
		runLocked: func(ctx context.Context, cfg config.Config) error {
			manager := New(cfg)
			return manager.restartMihomo(ctx)
		},
	})
}

// StopConfig loads runtime-safe paths and applied-state controls only after
// taking the lifecycle lock, matching the other path-based transitions.
func StopConfig(ctx context.Context, configPath string) error {
	return runConfigLifecycle(ctx, configPath, configLifecycleDeps{
		loadRuntime: config.LoadRecovery,
		loadLocked:  config.LoadRecovery,
		runLocked: func(ctx context.Context, cfg config.Config) error {
			manager := New(cfg)
			return manager.stop(ctx)
		},
	})
}

func runConfigLifecycle(ctx context.Context, configPath string, deps configLifecycleDeps) error {
	lockConfig, err := deps.loadRuntime(configPath)
	if err != nil {
		return err
	}
	return runtime.WithLifecycleLock(lockConfig, func() error {
		desired, err := deps.loadLocked(configPath)
		if err != nil {
			return err
		}
		if filepath.Clean(desired.Runtime.Dir) != filepath.Clean(lockConfig.Runtime.Dir) {
			return fmt.Errorf("runtime.dir changed while a gateway lifecycle action was waiting; retry the action")
		}
		return deps.runLocked(ctx, desired)
	})
}
