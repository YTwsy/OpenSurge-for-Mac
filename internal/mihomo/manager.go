package mihomo

import (
	"bytes"
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
	rendered, err := RenderConfig(m.cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(m.paths.MihomoConfig, []byte(rendered), 0o644)
}

func (m Manager) Start() (int, error) {
	binary, err := resolveBinary(m.cfg.Mihomo.Binary)
	if err != nil {
		return 0, err
	}
	if err := m.WriteConfig(); err != nil {
		return 0, err
	}
	if err := validateConfig(binary, m.paths.MihomoConfig); err != nil {
		return 0, err
	}
	if err := os.WriteFile(m.paths.MihomoLog, nil, 0o644); err != nil {
		return 0, err
	}
	return process.StartDetachedWithLog(m.paths.MihomoLog, binary, "-f", m.paths.MihomoConfig)
}

func (m Manager) Check() error {
	_, err := resolveBinary(m.cfg.Mihomo.Binary)
	return err
}

func validateConfig(binary string, configPath string) error {
	cmd := exec.Command(binary, "-t", "-f", configPath)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mihomo config validation failed: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
}

func (m Manager) Stop(pid int) error {
	return process.StopPID(pid, 3*time.Second)
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
		return "", fmt.Errorf("mihomo not found in PATH")
	}
	return binary, nil
}
