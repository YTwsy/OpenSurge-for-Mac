package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/dhcp"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/pf"
	"open-mihomo-gateway/internal/runtime"
	"open-mihomo-gateway/internal/sysctl"
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
	if _, exists, err := runtime.LoadState(m.paths.StateFile); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("gateway state already exists; run stop first")
	}
	if err := runtime.Ensure(m.paths); err != nil {
		return err
	}

	dhcpManager := dhcp.New(m.cfg, m.paths)
	mihomoManager := mihomo.New(m.cfg, m.paths)
	pfManager := pf.New(m.cfg, m.paths)
	sysctlManager := sysctl.New()
	if err := m.preflight(dhcpManager, mihomoManager, pfManager, sysctlManager); err != nil {
		return err
	}
	if err := mihomoManager.WriteConfig(); err != nil {
		return err
	}
	if err := dhcpManager.WriteConfig(); err != nil {
		return err
	}
	if err := pfManager.WriteAnchor(); err != nil {
		return err
	}

	ipForwardingBefore, err := sysctlManager.Current()
	if err != nil {
		return err
	}
	pfEnabledBefore, err := pfManager.Enabled()
	if err != nil {
		return err
	}

	state := runtime.State{
		StartedAt:          time.Now(),
		IPForwardingBefore: ipForwardingBefore,
		PFEnabledBefore:    pfEnabledBefore,
	}
	if err := runtime.SaveState(m.paths.StateFile, state); err != nil {
		return err
	}

	if err := sysctlManager.Enable(); err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}

	mihomoPID, err := mihomoManager.Start()
	if err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}
	state.PIDMihomo = mihomoPID
	if err := runtime.SaveState(m.paths.StateFile, state); err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}

	pid, err := dhcpManager.Start()
	if err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}
	state.PIDDNSMasq = pid
	if err := runtime.SaveState(m.paths.StateFile, state); err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}

	if err := pfManager.Load(!pfEnabledBefore); err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}
	state.PFAnchorLoaded = true
	if err := runtime.SaveState(m.paths.StateFile, state); err != nil {
		return m.rollback(err, state, dhcpManager, mihomoManager, pfManager, sysctlManager)
	}

	fmt.Printf("Gateway runtime prepared in %s\n", m.paths.Dir)
	if mihomoPID > 0 {
		fmt.Printf("mihomo started with pid %d\n", mihomoPID)
	}
	if pid > 0 {
		fmt.Printf("dnsmasq started with pid %d\n", pid)
	}
	fmt.Printf("pf anchor %s loaded\n", m.cfg.PF.AnchorName)
	return nil
}

func (m Manager) Stop(_ context.Context) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("stop requires sudo/root privileges")
	}
	state, exists, err := runtime.LoadState(m.paths.StateFile)
	if err != nil {
		return err
	}
	var cleanupErr error
	pfManager := pf.New(m.cfg, m.paths)
	sysctlManager := sysctl.New()
	if exists {
		dhcpManager := dhcp.New(m.cfg, m.paths)
		cleanupErr = errors.Join(cleanupErr, dhcpManager.Stop(state.PIDDNSMasq))
		mihomoManager := mihomo.New(m.cfg, m.paths)
		cleanupErr = errors.Join(cleanupErr, mihomoManager.Stop(state.PIDMihomo))
		if state.PFAnchorLoaded {
			cleanupErr = errors.Join(cleanupErr, pfManager.Unload(!state.PFEnabledBefore))
		}
		cleanupErr = errors.Join(cleanupErr, sysctlManager.Restore(state.IPForwardingBefore))
	}
	cleanupErr = errors.Join(cleanupErr, runtime.RemoveState(m.paths.StateFile))
	if cleanupErr != nil {
		return cleanupErr
	}

	fmt.Println("Gateway stopped and runtime state cleared.")
	return nil
}

func (m Manager) preflight(dhcpManager dhcp.Manager, mihomoManager mihomo.Manager, pfManager pf.Manager, sysctlManager sysctl.Manager) error {
	if err := dhcpManager.Check(); err != nil {
		return err
	}
	if err := mihomoManager.Check(); err != nil {
		return err
	}
	if err := pfManager.Check(); err != nil {
		return err
	}
	if err := sysctlManager.Check(); err != nil {
		return err
	}
	if _, err := net.InterfaceByName(m.cfg.Gateway.Interface); err != nil {
		return fmt.Errorf("interface %s: %w", m.cfg.Gateway.Interface, err)
	}
	if _, err := net.InterfaceByName(m.cfg.Gateway.UpstreamInterface); err != nil {
		return fmt.Errorf("upstream interface %s: %w", m.cfg.Gateway.UpstreamInterface, err)
	}
	return m.checkLANIP()
}

func (m Manager) checkLANIP() error {
	target := net.ParseIP(m.cfg.Gateway.LANIP).To4()
	if target == nil {
		return fmt.Errorf("gateway LAN IP %s is not IPv4", m.cfg.Gateway.LANIP)
	}
	iface, err := net.InterfaceByName(m.cfg.Gateway.Interface)
	if err != nil {
		return err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return err
	}
	for _, addr := range addrs {
		switch value := addr.(type) {
		case *net.IPNet:
			if value.IP.To4() != nil && value.IP.Equal(target) {
				return nil
			}
		case *net.IPAddr:
			if value.IP.To4() != nil && value.IP.Equal(target) {
				return nil
			}
		}
	}
	return fmt.Errorf("LAN IP %s is not configured on interface %s", m.cfg.Gateway.LANIP, m.cfg.Gateway.Interface)
}

func (m Manager) rollback(cause error, state runtime.State, dhcpManager dhcp.Manager, mihomoManager mihomo.Manager, pfManager pf.Manager, sysctlManager sysctl.Manager) error {
	var cleanupErr error
	cleanupErr = errors.Join(cleanupErr, dhcpManager.Stop(state.PIDDNSMasq))
	cleanupErr = errors.Join(cleanupErr, mihomoManager.Stop(state.PIDMihomo))
	if state.PFAnchorLoaded {
		cleanupErr = errors.Join(cleanupErr, pfManager.Unload(!state.PFEnabledBefore))
	}
	cleanupErr = errors.Join(cleanupErr, sysctlManager.Restore(state.IPForwardingBefore))
	cleanupErr = errors.Join(cleanupErr, runtime.RemoveState(m.paths.StateFile))
	if cleanupErr != nil {
		return fmt.Errorf("%w; rollback failed: %v", cause, cleanupErr)
	}
	return cause
}
