package gateway

import (
	"context"
	"fmt"
	"os"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/dhcp"
	"open-mihomo-gateway/internal/mihomo"
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
	if os.Geteuid() != 0 {
		return fmt.Errorf("start requires sudo/root privileges")
	}
	if err := runtime.Ensure(m.paths); err != nil {
		return err
	}

	state := runtime.State{
		StartedAt: time.Now(),
	}
	mihomoManager := mihomo.New(m.cfg, m.paths)
	mihomoPID, err := mihomoManager.Start()
	if err != nil {
		return err
	}
	state.PIDMihomo = mihomoPID

	dhcpManager := dhcp.New(m.cfg, m.paths)
	pid, err := dhcpManager.Start()
	if err != nil {
		_ = mihomoManager.Stop(mihomoPID)
		return err
	}
	state.PIDDNSMasq = pid

	if err := runtime.SaveState(m.paths.StateFile, state); err != nil {
		_ = dhcpManager.Stop(pid)
		_ = mihomoManager.Stop(mihomoPID)
		return err
	}

	fmt.Printf("Gateway runtime prepared in %s\n", m.paths.Dir)
	if mihomoPID > 0 {
		fmt.Printf("mihomo started with pid %d\n", mihomoPID)
	}
	if pid > 0 {
		fmt.Printf("dnsmasq started with pid %d\n", pid)
	}
	return nil
}

func (m Manager) Stop(_ context.Context) error {
	state, exists, err := runtime.LoadState(m.paths.StateFile)
	if err != nil {
		return err
	}
	if exists {
		dhcpManager := dhcp.New(m.cfg, m.paths)
		if err := dhcpManager.Stop(state.PIDDNSMasq); err != nil {
			return err
		}
		mihomoManager := mihomo.New(m.cfg, m.paths)
		if err := mihomoManager.Stop(state.PIDMihomo); err != nil {
			return err
		}
	}
	if err := runtime.RemoveState(m.paths.StateFile); err != nil {
		return err
	}

	fmt.Println("Gateway stopped and runtime state cleared.")
	return nil
}
