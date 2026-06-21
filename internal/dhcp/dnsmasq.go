package dhcp

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/process"
	"open-mihomo-gateway/internal/runtime"
)

type Manager struct {
	cfg   config.Config
	paths runtime.Paths
}

func New(cfg config.Config, paths runtime.Paths) Manager {
	return Manager{cfg: cfg, paths: paths}
}

func (m Manager) WriteConfig() error {
	rendered, err := RenderConfig(m.cfg, m.paths)
	if err != nil {
		return err
	}
	return os.WriteFile(m.paths.DNSMasqConf, []byte(rendered), 0o644)
}

func (m Manager) Start() (int, error) {
	if !m.cfg.DHCP.Enabled {
		return 0, nil
	}
	if _, err := exec.LookPath("dnsmasq"); err != nil {
		return 0, fmt.Errorf("dnsmasq not found in PATH")
	}
	if err := m.WriteConfig(); err != nil {
		return 0, err
	}
	return process.StartDetached("dnsmasq", "--keep-in-foreground", "--conf-file="+m.paths.DNSMasqConf)
}

func (m Manager) Stop(pid int) error {
	if err := process.StopPID(pid, 3*time.Second); err != nil {
		return err
	}
	_ = os.Remove(m.paths.DNSMasqPIDFile)
	return nil
}

func (m Manager) Running(pid int) bool {
	return process.IsAlive(pid)
}
