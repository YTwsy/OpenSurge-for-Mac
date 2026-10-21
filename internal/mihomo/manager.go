package mihomo

import (
	"bytes"
	"context"
	"errors"
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
	cfg     config.Config
	paths   runtime.Paths
	stopPID func(int, time.Duration) error
}

const (
	configValidationTimeout   = 90 * time.Second
	apiStartupTimeout         = 10 * time.Second
	tunStartupTimeout         = 10 * time.Second
	startupProcessStopTimeout = 3 * time.Second
)

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
	return m.ValidateWrittenConfigContext(context.Background())
}

func (m Manager) ValidateWrittenConfigContext(ctx context.Context) error {
	binary, err := resolveBinary(m.cfg.Mihomo.Binary)
	if err != nil {
		return err
	}
	return validateConfigContext(ctx, configValidationTimeout, binary, m.configDir(), m.paths.MihomoConfig)
}

func (m Manager) Start() (int, error) {
	return m.StartContext(context.Background())
}

// StartContext bounds readiness by the lifecycle request without tying the
// successfully started, detached process to that request's lifetime.
func (m Manager) StartContext(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	binary, err := resolveBinary(m.cfg.Mihomo.Binary)
	if err != nil {
		return 0, err
	}
	configDir := m.configDir()
	rendered, err := os.ReadFile(m.paths.MihomoConfig)
	if err != nil {
		return 0, fmt.Errorf("prepared mihomo config: %w", err)
	}
	dnsProbe, err := finalDNSProbeName(string(rendered))
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(m.paths.MihomoLog, nil, 0o640); err != nil {
		return 0, err
	}
	args, err := engineConfigArgs(configDir, m.paths.MihomoConfig, false)
	if err != nil {
		return 0, err
	}
	pid, err := process.StartDetachedWithLog(m.paths.MihomoLog, binary, args...)
	if err != nil {
		return 0, err
	}
	settledAt := time.Now().Add(300 * time.Millisecond)
	if err := m.waitForReadiness(ctx, pid, "process", apiStartupTimeout, time.Second, func(context.Context) (bool, error) {
		return !time.Now().Before(settledAt), nil
	}); err != nil {
		return m.failedStart(pid, err)
	}
	if err := m.waitForAPI(ctx, pid, apiStartupTimeout); err != nil {
		return m.failedStart(pid, err)
	}
	if dnsProbe != "" {
		if err := m.waitForDNS(ctx, pid, dnsProbe, 10*time.Second); err != nil {
			return m.failedStart(pid, err)
		}
	}
	if m.cfg.Transparent.TUNEnabled() {
		if err := m.waitForTUN(ctx, pid, tunStartupTimeout); err != nil {
			return m.failedStart(pid, err)
		}
	}
	if m.cfg.Transparent.TUNIPv6 != config.TUNIPv6Off {
		if err := m.waitForIPv6PacketListener(ctx, pid, tunStartupTimeout); err != nil {
			return m.failedStart(pid, err)
		}
	}
	return pid, nil
}

func (m Manager) waitForDNS(ctx context.Context, pid int, probeName string, timeout time.Duration) error {
	return m.waitForReadiness(ctx, pid, "DNS", timeout, 2*time.Second, func(probeCtx context.Context) (bool, error) {
		err := verifyPreparedDNSSockets(probeCtx, PreparedState{PID: pid, DNSAddr: "*:1053"})
		if err == nil {
			for _, network := range []string{"udp", "tcp"} {
				err = queryPreparedDNS(probeCtx, network, gatewayDNSEndpoint, probeName)
				if err != nil {
					break
				}
			}
		}
		return err == nil, err
	})
}

func (m Manager) waitForIPv6PacketListener(ctx context.Context, pid int, timeout time.Duration) error {
	return m.waitForReadiness(ctx, pid, "IPv6 packet listener", timeout, timeout, func(context.Context) (bool, error) {
		data, _ := os.ReadFile(m.paths.MihomoLog)
		logText := string(data)
		if strings.Contains(logText, "OpenSurge packet listener ready:") {
			return true, nil
		}
		if detail := ipv6PacketStartupError(logText); detail != "" {
			return true, fmt.Errorf("mihomo IPv6 packet listener failed to start: %s", detail)
		}
		return false, fmt.Errorf("listener has not reported ready")
	})
}

func ipv6PacketStartupError(logText string) string {
	marker := "Listener " + config.IPv6PacketListenerName + " listen err:"
	var detail string
	for _, line := range strings.Split(logText, "\n") {
		if index := strings.Index(line, marker); index >= 0 {
			detail = strings.TrimSpace(line[index+len(marker):])
		}
	}
	return detail
}

func (m Manager) failedStart(pid int, cause error) (int, error) {
	if err := m.stopStartedProcess(pid); err != nil {
		// Return ownership to the gateway so failed cleanup cannot orphan a
		// live engine while the runtime is reported stopped.
		return pid, errors.Join(cause, fmt.Errorf("cleanup failed mihomo startup: %w", err))
	}
	return 0, cause
}

func (m Manager) stopStartedProcess(pid int) error {
	stopPID := m.stopPID
	if stopPID == nil {
		stopPID = process.StopPID
	}
	return errors.Join(stopPID(pid, startupProcessStopTimeout), m.cleanupIPv6PacketSocket())
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
	return validateConfigContext(context.Background(), timeout, binary, configDir, configPath)
}

func validateConfigContext(ctx context.Context, timeout time.Duration, binary string, configDir string, configPath string) error {
	var output bytes.Buffer
	args, err := engineConfigArgs(configDir, configPath, true)
	if err != nil {
		return err
	}
	if err := process.RunBufferedContext(ctx, timeout, &output, binary, args...); err != nil {
		return fmt.Errorf("mihomo config validation failed: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
}

func (m Manager) waitForAPI(ctx context.Context, pid int, timeout time.Duration) error {
	return m.waitForReadiness(ctx, pid, "API", timeout, 250*time.Millisecond, func(ctx context.Context) (bool, error) {
		version, err := FetchVersion(ctx, m.cfg)
		if err == nil && strings.TrimSpace(version.Version) == "" {
			err = fmt.Errorf("controller returned an empty version")
		}
		return err == nil, err
	})
}

func (m Manager) waitForTUN(ctx context.Context, pid int, timeout time.Duration) error {
	return m.waitForReadiness(ctx, pid, "TUN", timeout, 300*time.Millisecond, func(ctx context.Context) (bool, error) {
		state, err := FetchTUNRuntimeState(ctx, m.cfg)
		if err == nil && state.Enabled {
			if m.cfg.Transparent.TUNAutoRoute {
				err = macosnetwork.VerifyMacTUNRoutes(ctx, state.Device, m.cfg.Transparent.MacTUNIPv6Enabled())
			}
			if err == nil {
				return true, nil
			}
		}
		if detail := tunStartupError(m.paths.MihomoLog); detail != "" {
			detail = enrichTUNRouteError(detail)
			return true, fmt.Errorf("mihomo TUN failed to start: %s", detail)
		}
		if err == nil {
			err = fmt.Errorf("runtime config still reports TUN disabled")
		}
		return false, err
	})
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
	stopPID := m.stopPID
	if stopPID == nil {
		stopPID = process.StopPID
	}
	return errors.Join(stopPID(pid, 3*time.Second), m.cleanupIPv6PacketSocket())
}

func (m Manager) cleanupIPv6PacketSocket() error {
	if m.cfg.Transparent.TUNIPv6 == config.TUNIPv6Off {
		return nil
	}
	info, err := os.Lstat(m.paths.IPv6PacketSocket)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to remove non-socket IPv6 packet listener path %s", m.paths.IPv6PacketSocket)
	}
	return os.Remove(m.paths.IPv6PacketSocket)
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
