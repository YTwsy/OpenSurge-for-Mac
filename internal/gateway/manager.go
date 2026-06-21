package gateway

import (
	"context"
	"fmt"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

type Manager struct {
	cfg   config.Config
	paths runtime.Paths
}

func New(cfg config.Config) Manager {
	return Manager{cfg: cfg, paths: runtime.NewPaths(cfg)}
}

func (m Manager) Start(_ context.Context) error {
	if err := runtime.Ensure(m.paths); err != nil {
		return err
	}

	state := runtime.State{
		StartedAt:           time.Now(),
		RuntimePreparedOnly: true,
	}
	if err := runtime.SaveState(m.paths.StateFile, state); err != nil {
		return err
	}

	fmt.Printf("Gateway runtime prepared in %s\n", m.paths.Dir)
	fmt.Println("Service startup will be enabled by later milestones.")
	return nil
}

func (m Manager) Stop(_ context.Context) error {
	if err := runtime.RemoveState(m.paths.StateFile); err != nil {
		return err
	}

	fmt.Println("Gateway runtime state cleared.")
	return nil
}
