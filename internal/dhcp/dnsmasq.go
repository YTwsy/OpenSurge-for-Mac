package dhcp

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
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
	if err := m.Check(); err != nil {
		return 0, err
	}
	if err := m.WriteConfig(); err != nil {
		return 0, err
	}
	binary, err := resolveBinary(m.cfg.DHCP.Binary)
	if err != nil {
		return 0, err
	}
	pid, err := process.StartDetached(binary, "--keep-in-foreground", "--conf-file="+m.paths.DNSMasqConf)
	if err != nil {
		return 0, err
	}
	if err := process.RequireAlive(pid, 300*time.Millisecond); err != nil {
		_ = process.StopPID(pid, 0)
		return 0, err
	}
	return pid, nil
}

func (m Manager) Check() error {
	if !m.cfg.DHCP.Enabled {
		return nil
	}
	_, err := resolveBinary(m.cfg.DHCP.Binary)
	return err
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

func resolveBinary(path string) (string, error) {
	if strings.ContainsRune(path, os.PathSeparator) {
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", fmt.Errorf("%s is a directory", path)
		}
		return path, nil
	}
	binary, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("dnsmasq not found in PATH")
	}
	return binary, nil
}
