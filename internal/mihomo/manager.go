package mihomo

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/macosnetwork"
	"open-mihomo-gateway/internal/process"
	"open-mihomo-gateway/internal/runtime"
)

type Manager struct {
	cfg   config.Config
	paths runtime.Paths
}

const configValidationTimeout = 90 * time.Second

func New(cfg config.Config, paths runtime.Paths) Manager {
	return Manager{cfg: cfg, paths: paths}
}

func (m Manager) WriteConfig() error {
	rendered, err := RenderConfig(m.cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(m.paths.MihomoConfig, []byte(rendered), 0o640)
}

func (m Manager) ValidateConfig() error {
	if err := runtime.Ensure(m.paths); err != nil {
		return err
	}
	if err := m.WriteConfig(); err != nil {
		return err
	}
	return m.ValidateWrittenConfig()
}

// ValidateWrittenConfig validates the already-rendered configuration. Gateway
// startup calls this before enabling forwarding, and Start deliberately does
// not re-read or re-render policy input afterwards.
func (m Manager) ValidateWrittenConfig() error {
	binary, err := resolveBinary(m.cfg.Mihomo.Binary)
	if err != nil {
		return err
	}
	return validateConfig(binary, m.configDir(), m.paths.MihomoConfig)
}

func (m Manager) Start() (int, error) {
	binary, err := resolveBinary(m.cfg.Mihomo.Binary)
	if err != nil {
		return 0, err
	}
	configDir := m.configDir()
	if _, err := os.Stat(m.paths.MihomoConfig); err != nil {
		return 0, fmt.Errorf("prepared mihomo config: %w", err)
	}
	if err := os.WriteFile(m.paths.MihomoLog, nil, 0o640); err != nil {
		return 0, err
	}
	pid, err := process.StartDetachedWithLog(m.paths.MihomoLog, binary, "-d", configDir, "-f", m.paths.MihomoConfig)
	if err != nil {
		return 0, err
	}
	if err := process.RequireAlive(pid, 300*time.Millisecond); err != nil {
		_ = process.StopPID(pid, 0)
		return 0, err
	}
	if err := m.waitForAPI(pid, 2*time.Second); err != nil {
		_ = process.StopPID(pid, 0)
		return 0, err
	}
	if m.cfg.Transparent.TUNEnabled() {
		if err := m.waitForTUN(pid, 3*time.Second); err != nil {
			_ = process.StopPID(pid, 0)
			return 0, err
		}
	}
	return pid, nil
}

func (m Manager) Check() error {
	_, err := resolveBinary(m.cfg.Mihomo.Binary)
	return err
}

func (m Manager) configDir() string {
	if m.cfg.Mihomo.ProfileMode == config.MihomoProfileModeImported && strings.TrimSpace(m.cfg.Mihomo.Profile) != "" {
		return filepath.Dir(m.cfg.Mihomo.Profile)
	}
	return filepath.Dir(m.paths.MihomoConfig)
}

func validateConfig(binary string, configDir string, configPath string) error {
	return validateConfigWithTimeout(configValidationTimeout, binary, configDir, configPath)
}

func validateConfigWithTimeout(timeout time.Duration, binary string, configDir string, configPath string) error {
	var output bytes.Buffer
	if err := process.RunBufferedTimeout(timeout, &output, binary, "-d", configDir, "-t", "-f", configPath); err != nil {
		return fmt.Errorf("mihomo config validation failed: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
}

func (m Manager) waitForAPI(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if !process.IsAlive(pid) {
			return fmt.Errorf("mihomo pid %d exited during startup", pid)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		_, err := FetchVersion(ctx, m.cfg)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("mihomo API not ready after %s: %w", timeout, lastErr)
	}
	return fmt.Errorf("mihomo API not ready after %s", timeout)
}

func (m Manager) waitForTUN(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if !process.IsAlive(pid) {
			return fmt.Errorf("mihomo pid %d exited while TUN was starting", pid)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		state, err := FetchTUNRuntimeState(ctx, m.cfg)
		cancel()
		if err == nil && state.Enabled {
			return nil
		}
		if err != nil {
			lastErr = err
		}
		if detail := tunStartupError(m.paths.MihomoLog); detail != "" {
			detail = enrichTUNRouteError(detail)
			return fmt.Errorf("mihomo TUN failed to start: %s", detail)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("mihomo TUN not ready after %s: %w", timeout, lastErr)
	}
	return fmt.Errorf("mihomo TUN not ready after %s: runtime config still reports disabled", timeout)
}

func enrichTUNRouteError(detail string) string {
	const marker = "add route:"
	index := strings.Index(detail, marker)
	if index < 0 {
		return detail
	}
	value := strings.TrimSpace(detail[index+len(marker):])
	prefix, _, ok := strings.Cut(value, ":")
	if !ok {
		return detail
	}
	prefix = strings.TrimSpace(prefix)
	ip, _, err := net.ParseCIDR(prefix)
	if err != nil {
		return detail
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	route, err := macosnetwork.LookupRoute(ctx, ip.String())
	if err != nil || route.Interface == "" {
		return detail
	}
	if route.Gateway != "" {
		return fmt.Sprintf("%s; existing route selects %s via %s", detail, route.Interface, route.Gateway)
	}
	return fmt.Sprintf("%s; existing route selects %s", detail, route.Interface)
}

func tunStartupError(logPath string) string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	const marker = "Start TUN listening error:"
	var detail string
	for _, line := range strings.Split(string(data), "\n") {
		if index := strings.Index(line, marker); index >= 0 {
			detail = strings.TrimSpace(line[index+len(marker):])
		}
	}
	return detail
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
