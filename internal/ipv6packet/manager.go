package ipv6packet

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

func NewManager(cfg config.Config, paths runtime.Paths) Manager {
	return Manager{cfg: cfg, paths: paths}
}

func (m Manager) ResolveBinary() (string, error) {
	return resolveBrokerBinary(m.cfg.Transparent.IPv6PacketBrokerBinary, m.cfg.Runtime.Dir)
}

func (m Manager) Check() error {
	_, err := m.ResolveBinary()
	return err
}

func (m Manager) Start() (int, error) {
	binary, err := m.ResolveBinary()
	if err != nil {
		return 0, err
	}
	_ = os.Remove(m.paths.IPv6PacketReady)
	if err := os.WriteFile(m.paths.IPv6PacketLog, nil, 0o640); err != nil {
		return 0, err
	}
	pid, err := process.StartDetachedWithLog(
		m.paths.IPv6PacketLog,
		binary,
		config.IPv6PacketBrokerSubcommand,
		"--interface", m.cfg.Gateway.Interface,
		"--socket", m.paths.IPv6PacketSocket,
		"--ready-file", m.paths.IPv6PacketReady,
		"--mtu", strconv.Itoa(m.cfg.Transparent.IPv6PacketMTU),
	)
	if err != nil {
		return 0, err
	}
	if err := process.RequireAlive(pid, 300*time.Millisecond); err != nil {
		_ = process.StopPID(pid, time.Second)
		return 0, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !process.IsAlive(pid) {
			return 0, fmt.Errorf("IPv6 packet broker pid %d exited during startup", pid)
		}
		if info, err := os.Stat(m.paths.IPv6PacketReady); err == nil && info.Mode().IsRegular() {
			return pid, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = process.StopPID(pid, time.Second)
	return 0, fmt.Errorf("IPv6 packet broker did not become ready after 5s: %s", strings.TrimSpace(readBrokerLog(m.paths.IPv6PacketLog)))
}

func (m Manager) Stop(pid int) error {
	err := process.StopPID(pid, 3*time.Second)
	_ = os.Remove(m.paths.IPv6PacketReady)
	_ = os.Remove(m.paths.IPv6PacketSocket + ".broker")
	return err
}

func (m Manager) Running(pid int) bool { return process.IsAlive(pid) }

func readBrokerLog(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func resolveBrokerBinary(path, runtimeDir string) (string, error) {
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
	if binary, err := exec.LookPath(path); err == nil {
		return binary, nil
	}
	if executable, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), path)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	// Installed configurations keep mutable runtime state under
	// <OpenSurge root>/runtime and packaged executables under the sibling bin
	// directory. The root helper is launched from /Library/PrivilegedHelperTools,
	// so the broker is not beside that executable and launchd's PATH does not
	// include the product-owned bin directory.
	if runtimeDir != "" {
		candidate := filepath.Join(filepath.Dir(runtimeDir), "bin", path)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s not found in PATH, beside the OpenSurge executable, or in the installed bin directory", path)
}
