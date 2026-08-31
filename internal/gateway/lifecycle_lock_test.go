package gateway

import (
	"errors"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestLifecycleLockExcludesConcurrentGatewayOperations(t *testing.T) {
	cfg := config.Config{Runtime: config.RuntimeConfig{Dir: t.TempDir()}}
	first, err := acquireLifecycleLock(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.release() })

	busy, err := LifecycleOperationInProgress(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !busy {
		t.Fatal("held lifecycle lock was reported as available")
	}
	if _, err := acquireLifecycleLock(cfg); !errors.Is(err, ErrLifecycleOperationInProgress) {
		t.Fatalf("second lifecycle lock error = %v", err)
	}

	if err := first.release(); err != nil {
		t.Fatal(err)
	}
	busy, err = LifecycleOperationInProgress(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if busy {
		t.Fatal("released lifecycle lock remained busy")
	}
}

func TestLifecycleOperationInProgressWithoutLockFile(t *testing.T) {
	cfg := config.Config{Runtime: config.RuntimeConfig{Dir: t.TempDir()}}
	busy, err := LifecycleOperationInProgress(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if busy {
		t.Fatal("missing lifecycle lock file was reported as busy")
	}
}
