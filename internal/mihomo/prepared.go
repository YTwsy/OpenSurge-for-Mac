package mihomo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/process"
	"open-mihomo-gateway/internal/runtime"
)

const preparedDirectoryName = "prepared-mihomo"

var ErrGatewayOwnsEngine = errors.New("gateway runtime exists; prepared engine is unavailable")

// PreparedState is a private ownership record, not a public API response. The
// controller secret must never be included in a Web GUI response or diagnostic.
type PreparedState struct {
	PID                    int                      `json:"pid"`
	ProcessFingerprint     string                   `json:"process_fingerprint"`
	BootSessionID          string                   `json:"boot_session_id"`
	StartedAt              time.Time                `json:"started_at"`
	ConfigDigest           string                   `json:"config_digest"`
	ConfigDirectory        string                   `json:"config_directory"`
	APIAddr                string                   `json:"api_addr"`
	Secret                 string                   `json:"secret"`
	DNSAddr                string                   `json:"dns_addr,omitempty"`
	DNSProbeName           string                   `json:"dns_probe_name,omitempty"`
	DevicePolicyResolution *device.PolicyResolution `json:"device_policy_resolution,omitempty"`
}

type preparedDeps struct {
	boot         func() (runtime.BootSession, error)
	fingerprint  func(int) (string, error)
	matches      func(int, string) (bool, error)
	start        func(string, string, string, string) (int, error)
	stop         func(int, time.Duration) error
	validate     func(string, string, string) error
	version      func(context.Context, config.Config) (Version, error)
	reserveDNS   func() (string, func(), error)
	dnsReady     func(context.Context, PreparedState) error
	readyTimeout time.Duration
}

func defaultPreparedDeps() preparedDeps {
	return preparedDeps{
		boot:        runtime.CurrentBootSession,
		fingerprint: process.Fingerprint,
		matches:     process.MatchesFingerprint,
		start: func(logPath, binary, directory, path string) (int, error) {
			args, err := engineConfigArgs(directory, path, false)
			if err != nil {
				return 0, err
			}
			return process.StartDetachedWithLog(logPath, binary, args...)
		},
		stop:       process.StopPID,
		validate:   validateConfig,
		version:    FetchVersion,
		reserveDNS: reservePreparedDNS,
		dnsReady:   probePreparedDNS,
	}
}

// PreparedConfig supplies the private prepared controller to existing mihomo
// API helpers. It must not be persisted as the desired gateway configuration.
func PreparedConfig(cfg config.Config, state PreparedState) config.Config {
	cfg.Mihomo.APIAddr = state.APIAddr
	cfg.Mihomo.Secret = state.Secret
	return cfg
}

// Prepare loads the same final policy graph as gateway startup, without any
// host-network or business ingress. Callers that will use the controller must
// instead hold runtime.WithLifecycleLock across PrepareLocked and their API
// operation, so a concurrent CLI start cannot replace the prepared engine.
func Prepare(ctx context.Context, cfg config.Config, renderedFinal string) (state PreparedState, err error) {
	err = runtime.WithLifecycleLock(cfg, func() error {
		var prepareErr error
		state, prepareErr = PrepareLocked(ctx, cfg, renderedFinal)
		return prepareErr
	})
	return state, err
}

// PrepareLocked requires the shared runtime lifecycle lock. It never starts a
// second process while a gateway runtime (including interrupted state) exists.
func PrepareLocked(ctx context.Context, cfg config.Config, renderedFinal string) (PreparedState, error) {
	return prepareLocked(ctx, cfg, renderedFinal, defaultPreparedDeps())
}

func prepareLocked(ctx context.Context, cfg config.Config, renderedFinal string, deps preparedDeps) (PreparedState, error) {
	if err := ctx.Err(); err != nil {
		return PreparedState{}, err
	}
	if _, exists, err := runtime.LoadState(runtime.NewPaths(cfg).StateFile); err != nil {
		return PreparedState{}, err
	} else if exists {
		return PreparedState{}, ErrGatewayOwnsEngine
	}
	boot, err := deps.boot()
	if err != nil {
		return PreparedState{}, fmt.Errorf("determine prepared engine boot session: %w", err)
	}
	manager := New(cfg, runtime.NewPaths(cfg))
	directory, err := filepath.Abs(manager.configDir())
	if err != nil {
		return PreparedState{}, err
	}
	binary, err := resolveBinary(cfg.Mihomo.Binary)
	if err != nil {
		return PreparedState{}, err
	}
	digest := sha256.Sum256([]byte(renderedFinal + "\x00" + directory + "\x00" + binary))
	wantDigest := hex.EncodeToString(digest[:])
	previous, exists, err := LoadPrepared(cfg)
	if err != nil {
		return PreparedState{}, err
	}
	if exists && preparedBelongsToBoot(previous, boot) && previous.ConfigDigest == wantDigest {
		alive, err := preparedProcessMatches(previous, deps)
		if err != nil {
			return PreparedState{}, err
		}
		if alive {
			probeCtx, cancel := context.WithTimeout(ctx, time.Second)
			_, err = deps.version(probeCtx, PreparedConfig(cfg, previous))
			cancel()
			if err != nil {
				// An alive-but-unreachable engine is not permission to spawn a
				// competing tsnet identity, nor an automatic recovery loop.
				return PreparedState{}, fmt.Errorf("prepared engine controller unavailable: %w", err)
			}
			if previous.DNSAddr != "" {
				probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				err := deps.dnsReady(probeCtx, previous)
				cancel()
				if err != nil {
					return PreparedState{}, &preparedDNSError{err}
				}
			}
			return previous, nil
		}
	}
	if err := stopPreparedLocked(cfg, deps); err != nil {
		return PreparedState{}, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		state, err := startPreparedAttempt(ctx, cfg, renderedFinal, directory, binary, wantDigest, boot, deps)
		var dnsErr *preparedDNSError
		if err == nil || !errors.As(err, &dnsErr) || ctx.Err() != nil || attempt == 2 {
			return state, err
		}
		// A failed DNS bind can leave a live controller. Never start a second
		// process until the exact prior owner and its listeners have exited.
		if err := stopPreparedLocked(cfg, deps); err != nil {
			return PreparedState{}, err
		}
	}
	return PreparedState{}, fmt.Errorf("prepared DNS retry limit reached")
}

func startPreparedAttempt(ctx context.Context, cfg config.Config, renderedFinal, directory, binary, wantDigest string, boot runtime.BootSession, deps preparedDeps) (PreparedState, error) {
	if err := ctx.Err(); err != nil {
		return PreparedState{}, err
	}
	apiAddr, secret, err := newPreparedControllerIdentity()
	if err != nil {
		return PreparedState{}, err
	}
	needsDNS, err := preparedNeedsDNS(renderedFinal)
	if err != nil {
		return PreparedState{}, err
	}
	dnsAddr, probeName := "", ""
	releaseDNS := func() {}
	if needsDNS {
		dnsAddr, releaseDNS, err = deps.reserveDNS()
		if err != nil {
			return PreparedState{}, &preparedDNSError{err}
		}
		defer releaseDNS()
		if dnsAddr == apiAddr {
			apiAddr, secret, err = newPreparedControllerIdentity()
			if err != nil {
				return PreparedState{}, err
			}
		}
		probeName, err = newDNSProbeName()
		if err != nil {
			return PreparedState{}, err
		}
	}
	rendered, err := renderPreparedConfig(renderedFinal, apiAddr, secret, dnsAddr, probeName)
	if err != nil {
		return PreparedState{}, err
	}
	preparedDir := filepath.Join(cfg.Runtime.Dir, preparedDirectoryName)
	if err := os.MkdirAll(preparedDir, 0o700); err != nil {
		return PreparedState{}, err
	}
	if err := os.Chmod(preparedDir, 0o700); err != nil {
		return PreparedState{}, err
	}
	configPath := filepath.Join(preparedDir, "mihomo.yaml")
	if err := writePreparedFile(configPath, []byte(rendered)); err != nil {
		return PreparedState{}, err
	}
	// The work/cache directory is deliberately identical to the real gateway's
	// directory. store-selected, providers and tsnet have one sequential owner;
	// no copied identity or second cache is introduced by the preview workspace.
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return PreparedState{}, err
	}
	if err := deps.validate(binary, directory, configPath); err != nil {
		return PreparedState{}, err
	}
	if err := ctx.Err(); err != nil {
		return PreparedState{}, err
	}
	logPath := filepath.Join(preparedDir, "mihomo.log")
	if err := writePreparedFile(logPath, nil); err != nil {
		return PreparedState{}, err
	}
	state := PreparedState{
		BootSessionID: boot.ID, StartedAt: time.Now().UTC(), ConfigDigest: wantDigest,
		ConfigDirectory: directory, APIAddr: apiAddr, Secret: secret,
		DNSAddr: dnsAddr, DNSProbeName: probeName,
	}
	if cfg.DevicePolicy.Bundle != nil {
		state.DevicePolicyResolution = cfg.DevicePolicy.Bundle.Resolution
	}
	// Journal intent before fork. If the Helper dies in the small interval
	// before the child's fingerprint is saved, an incomplete current-boot
	// record blocks a competing engine instead of silently losing ownership.
	if err := savePrepared(cfg, state); err != nil {
		return PreparedState{}, err
	}
	releaseDNS()
	state.PID, err = deps.start(logPath, binary, directory, configPath)
	if err != nil {
		return PreparedState{}, errors.Join(err, runtime.RemoveState(preparedStatePath(cfg)))
	}
	state.ProcessFingerprint, err = deps.fingerprint(state.PID)
	if err != nil || state.ProcessFingerprint == "" {
		stopErr := deps.stop(state.PID, startupProcessStopTimeout)
		if stopErr == nil {
			stopErr = runtime.RemoveState(preparedStatePath(cfg))
		}
		return PreparedState{}, errors.Join(fmt.Errorf("record prepared engine identity: %w", nonNilError(err, "empty process fingerprint")), stopErr)
	}
	if err := savePrepared(cfg, state); err != nil {
		stopErr := deps.stop(state.PID, startupProcessStopTimeout)
		if stopErr == nil {
			stopErr = runtime.RemoveState(preparedStatePath(cfg))
		}
		return PreparedState{}, errors.Join(err, stopErr)
	}
	if err := waitPreparedReady(ctx, cfg, state, deps); err != nil {
		return PreparedState{}, errors.Join(err, stopPreparedLocked(cfg, deps))
	}
	return state, nil
}

// RenderPreparedConfig keeps only outbound/rule/resolver fields of the final
// rendered configuration. All ingress and host-mutating fields are rebuilt
// from a closed allowlist; new mihomo listener options cannot become active
// merely because a future source or renderer starts including them.
func RenderPreparedConfig(renderedFinal, apiAddr, secret string) (string, error) {
	return renderPreparedConfig(renderedFinal, apiAddr, secret, "", "")
}

func renderPreparedConfig(renderedFinal, apiAddr, secret, dnsAddr, probeName string) (string, error) {
	host, _, err := net.SplitHostPort(apiAddr)
	if err != nil || host != "127.0.0.1" || strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("prepared controller requires authenticated IPv4 loopback")
	}
	root, err := decodeSingleYAMLMapping([]byte(renderedFinal))
	if err != nil {
		return "", fmt.Errorf("decode final prepared policy: %w", err)
	}
	if err := validateMappingKeys(root); err != nil {
		return "", err
	}
	allowed := map[string]bool{
		"mode": true, "log-level": true, "ipv6": true,
		"proxies": true, "proxy-groups": true, "proxy-providers": true,
		"rules": true, "rule-providers": true, "sub-rules": true,
		"dns": true, "hosts": true, "profile": true,
		"geodata-mode": true, "geodata-loader": true, "geox-url": true,
		"global-client-fingerprint": true, "global-ua": true,
		"unified-delay": true, "tcp-concurrent": true,
		"keep-alive-interval": true, "keep-alive-idle": true,
		"disable-keep-alive": true,
	}
	result := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if allowed[root.Content[i].Value] {
			result.Content = append(result.Content, root.Content[i], root.Content[i+1])
		}
	}
	setPreparedField(result, "external-controller", apiAddr)
	setPreparedField(result, "secret", secret)
	setPreparedField(result, "allow-lan", false)
	setPreparedField(result, "bind-address", "127.0.0.1")
	for _, field := range []string{"port", "socks-port", "mixed-port", "redir-port", "tproxy-port"} {
		setPreparedField(result, field, 0)
	}
	setPreparedField(result, "tun", map[string]any{"enable": false, "auto-route": false})
	setPreparedField(result, "listeners", []any{})
	setPreparedField(result, "tunnels", []any{})
	if err := bindPreparedDNS(result, renderedFinal, dnsAddr, probeName); err != nil {
		return "", err
	}
	if err := validateNodeAliases(result); err != nil {
		return "", err
	}
	data, err := yaml.Marshal(result)
	return string(data), err
}

func setPreparedField(mapping *yaml.Node, name string, value any) {
	var encoded yaml.Node
	_ = encoded.Encode(value) // values are fixed primitives and containers above
	if index := mappingValueIndex(mapping, name); index >= 0 {
		mapping.Content[index] = &encoded
	} else {
		mapping.Content = append(mapping.Content, stringNode(name), &encoded)
	}
}

func newPreparedControllerIdentity() (string, string, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", "", err
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return "", "", err
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", "", err
	}
	return address, hex.EncodeToString(secret[:]), nil
}

func waitPreparedReady(ctx context.Context, cfg config.Config, state PreparedState, deps preparedDeps) error {
	timeout := deps.readyTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	readyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var lastErr error
	dnsFailed := false
	for {
		alive, err := preparedProcessMatches(state, deps)
		if err != nil || !alive {
			return fmt.Errorf("prepared engine exited during startup: %w", nonNilError(err, "process identity no longer matches"))
		}
		probeCtx, probeCancel := context.WithTimeout(readyCtx, 300*time.Millisecond)
		_, lastErr = deps.version(probeCtx, PreparedConfig(cfg, state))
		probeCancel()
		dnsFailed = false
		if lastErr == nil && state.DNSAddr != "" {
			probeCtx, probeCancel := context.WithTimeout(readyCtx, 2*time.Second)
			lastErr = deps.dnsReady(probeCtx, state)
			probeCancel()
			dnsFailed = lastErr != nil
		}
		if lastErr == nil {
			return nil
		}
		select {
		case <-readyCtx.Done():
			if dnsFailed {
				return &preparedDNSError{errors.Join(readyCtx.Err(), lastErr)}
			}
			return fmt.Errorf("prepared engine API not ready: %w", errors.Join(readyCtx.Err(), lastErr))
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// LoadPrepared reads the ownership record only; existence is not proof of a
// live process. PrepareLocked/StopPreparedLocked verify boot and fingerprint.
func LoadPrepared(cfg config.Config) (PreparedState, bool, error) {
	data, err := os.ReadFile(preparedStatePath(cfg))
	if errors.Is(err, os.ErrNotExist) {
		return PreparedState{}, false, nil
	}
	if err != nil {
		return PreparedState{}, false, err
	}
	var state PreparedState
	if err := json.Unmarshal(data, &state); err != nil {
		return PreparedState{}, false, fmt.Errorf("decode prepared engine state: %w", err)
	}
	if state.BootSessionID == "" && state.StartedAt.IsZero() {
		return PreparedState{}, false, fmt.Errorf("prepared engine state lacks boot ownership; refusing untracked cleanup")
	}
	return state, true, nil
}

func StopPrepared(cfg config.Config) error {
	return runtime.WithLifecycleLock(cfg, func() error { return StopPreparedLocked(cfg) })
}

// StopPreparedLocked requires the shared lifecycle lock and must succeed before
// gateway startup can acquire the shared cache and embedded tsnet identity.
func StopPreparedLocked(cfg config.Config) error {
	return stopPreparedLocked(cfg, defaultPreparedDeps())
}

func stopPreparedLocked(cfg config.Config, deps preparedDeps) error {
	state, exists, err := LoadPrepared(cfg)
	if err != nil || !exists {
		return err
	}
	boot, err := deps.boot()
	if err != nil {
		return fmt.Errorf("verify prepared engine boot session: %w", err)
	}
	if preparedBelongsToBoot(state, boot) {
		alive, err := preparedProcessMatches(state, deps)
		if err != nil {
			return err
		}
		if alive {
			if err := deps.stop(state.PID, startupProcessStopTimeout); err != nil {
				return fmt.Errorf("stop prepared engine: %w", err)
			}
			// StopPID may have just sent SIGKILL. Do not let the next engine
			// acquire cache.db/tsnet until the exact former process is gone.
			deadline := time.Now().Add(2 * time.Second)
			for {
				alive, err = preparedProcessMatches(state, deps)
				if err != nil {
					return err
				}
				if !alive {
					break
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("prepared engine has not exited; ownership retained")
				}
				time.Sleep(20 * time.Millisecond)
			}
		}
	}
	// Old-boot and reused-PID records are stale, never a license to signal the
	// process currently occupying that PID. Preserve the durable cache/identity.
	if err := runtime.RemoveState(preparedStatePath(cfg)); err != nil {
		return err
	}
	return nil
}

func preparedProcessMatches(state PreparedState, deps preparedDeps) (bool, error) {
	if state.PID <= 0 || strings.TrimSpace(state.ProcessFingerprint) == "" {
		return false, fmt.Errorf("prepared engine state lacks a process identity; refusing untracked cleanup")
	}
	return deps.matches(state.PID, state.ProcessFingerprint)
}

func preparedBelongsToBoot(state PreparedState, boot runtime.BootSession) bool {
	return (runtime.State{BootSessionID: state.BootSessionID, StartedAt: state.StartedAt}).BelongsToBoot(boot)
}

func preparedStatePath(cfg config.Config) string {
	return filepath.Join(cfg.Runtime.Dir, preparedDirectoryName, "state.json")
}

func savePrepared(cfg config.Config, state PreparedState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writePreparedFile(preparedStatePath(cfg), append(data, '\n'))
}

func writePreparedFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".prepared-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func nonNilError(err error, message string) error {
	if err != nil {
		return err
	}
	return errors.New(message)
}
