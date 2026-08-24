package controlapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/lan"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

type profileApplyDeps struct {
	geteuid     func() int
	validate    func(config.Config) error
	stateExists func(config.Config) (bool, error)
	reload      func(context.Context, config.Config) error
	start       func(context.Context, config.Config) error
}

func defaultProfileApplyDeps() profileApplyDeps {
	return profileApplyDeps{
		geteuid: os.Geteuid,
		validate: func(cfg config.Config) error {
			temp, err := os.MkdirTemp(cfg.Runtime.Dir, ".profile-validation-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(temp)
			validation := cfg
			validation.Runtime.Dir = temp
			validation.Mihomo.Config = filepath.Join(temp, "mihomo.yaml")
			return mihomo.New(validation, runtime.NewPaths(validation)).ValidateConfig()
		},
		stateExists: func(cfg config.Config) (bool, error) {
			_, exists, err := runtime.LoadState(runtime.NewPaths(cfg).StateFile)
			return exists, err
		},
		reload: func(ctx context.Context, cfg config.Config) error { return gateway.New(cfg).Reload(ctx) },
		start:  func(ctx context.Context, cfg config.Config) error { return gateway.New(cfg).Start(ctx) },
	}
}

func (DirectRunner) ApplyProfile(ctx context.Context, configPath, revision string, payload []byte) (ProfileApplyResult, error) {
	return applyProfile(ctx, configPath, revision, payload, defaultProfileApplyDeps())
}

func applyProfile(ctx context.Context, configPath, revision string, payload []byte, deps profileApplyDeps) (ProfileApplyResult, error) {
	if deps.geteuid() != 0 {
		return ProfileApplyResult{}, fmt.Errorf("privileged helper is required")
	}
	if len(payload) == 0 || len(payload) > maxSourceSize {
		return ProfileApplyResult{}, fmt.Errorf("profile payload must be between 1 byte and 10 MiB")
	}
	if revision == "" || revision != fileDigest(configPath) {
		return ProfileApplyResult{}, fmt.Errorf("config revision conflict")
	}
	if _, err := inspectSource(payload, "mihomo_profile"); err != nil {
		return ProfileApplyResult{}, err
	}
	original, err := os.ReadFile(configPath)
	if err != nil {
		return ProfileApplyResult{}, err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return ProfileApplyResult{}, err
	}
	previousCfg := cfg
	wasRunning, err := deps.stateExists(cfg)
	if err != nil {
		return ProfileApplyResult{}, err
	}
	digest := sha256.Sum256(payload)
	profilePath := filepath.Join(filepath.Dir(configPath), "data", "imported-profile-"+hex.EncodeToString(digest[:8])+".yaml")
	_, statErr := os.Stat(profilePath)
	profileExisted := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return ProfileApplyResult{}, statErr
	}
	if err := writeAtomic(profilePath, payload, 0o640); err != nil {
		return ProfileApplyResult{}, err
	}
	cleanupProfile := func() {
		if !profileExisted {
			_ = os.Remove(profilePath)
		}
	}
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = profilePath
	if err := deps.validate(cfg); err != nil {
		cleanupProfile()
		return ProfileApplyResult{}, err
	}
	if err := writeAtomic(configPath, []byte(config.Render(cfg)), 0o640); err != nil {
		cleanupProfile()
		return ProfileApplyResult{}, err
	}
	result := ProfileApplyResult{Revision: fileDigest(configPath)}
	if !wasRunning {
		return result, nil
	}
	if err := deps.reload(ctx, cfg); err != nil {
		rollbackErr := writeAtomic(configPath, original, 0o640)
		var restartErr error
		if rollbackErr == nil {
			stillRunning, stateErr := deps.stateExists(previousCfg)
			if stateErr != nil {
				rollbackErr = fmt.Errorf("inspect gateway after failed reload: %w", stateErr)
			} else if !stillRunning {
				restartErr = deps.start(ctx, previousCfg)
			}
		}
		cleanupProfile()
		return ProfileApplyResult{}, profileApplyRollbackError(err, rollbackErr, restartErr)
	}
	result.Reloaded = true
	return result, nil
}

func (DirectRunner) ApplyTailscale(ctx context.Context, configPath, revision string, payload []byte) (ProfileApplyResult, error) {
	return applyTailscale(ctx, configPath, revision, payload, defaultProfileApplyDeps())
}

func applyTailscale(ctx context.Context, configPath, revision string, payload []byte, deps profileApplyDeps) (ProfileApplyResult, error) {
	if deps.geteuid() != 0 {
		return ProfileApplyResult{}, fmt.Errorf("privileged helper is required")
	}
	if len(payload) == 0 || len(payload) > 256<<10 {
		return ProfileApplyResult{}, fmt.Errorf("tailscale payload must be between 1 byte and 256 KiB")
	}
	if revision == "" || revision != fileDigest(configPath) {
		return ProfileApplyResult{}, fmt.Errorf("config revision conflict")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var input TailscaleUpdateRequest
	if err := decoder.Decode(&input); err != nil {
		return ProfileApplyResult{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return ProfileApplyResult{}, fmt.Errorf("expected one JSON document")
		}
		return ProfileApplyResult{}, err
	}
	if err := normalizeTailscaleUpdate(&input); err != nil {
		return ProfileApplyResult{}, err
	}
	original, err := os.ReadFile(configPath)
	if err != nil {
		return ProfileApplyResult{}, err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return ProfileApplyResult{}, err
	}
	previousCfg := cfg
	wasRunning, err := deps.stateExists(cfg)
	if err != nil {
		return ProfileApplyResult{}, err
	}
	authKeyPath, stateDir := tailscaleManagedPaths(configPath)
	authSnapshot, err := snapshotFile(authKeyPath)
	if err != nil {
		return ProfileApplyResult{}, err
	}
	if input.AuthKey != "" {
		if err := writeAtomic(authKeyPath, []byte(input.AuthKey+"\n"), 0o600); err != nil {
			return ProfileApplyResult{}, err
		}
	}
	cfg.Tailscale = config.TailscaleConfig{
		Enabled:                input.Enabled,
		DisplayName:            input.DisplayName,
		Hostname:               input.Hostname,
		ControlURL:             input.ControlURL,
		AuthKeyFile:            authKeyPath,
		StateDir:               stateDir,
		AcceptRoutes:           input.AcceptRoutes,
		MagicDNSSuffixes:       append([]string(nil), input.MagicDNSSuffixes...),
		PeerCIDRs:              append([]string(nil), input.PeerCIDRs...),
		SubnetRoutes:           append([]string(nil), input.SubnetRoutes...),
		AllowMac:               input.AllowMac,
		AllowAllDevices:        input.AllowAllDevices,
		AllowedDevices:         append([]string(nil), input.AllowedDevices...),
		ExitNode:               input.ExitNode,
		ExitNodeAllowLANAccess: input.ExitNodeAllowLANAccess,
	}
	rollbackKey := func() { _ = restoreFile(authKeyPath, authSnapshot) }
	if input.Enabled && input.AuthKey == "" {
		if _, err := os.Stat(authKeyPath); os.IsNotExist(err) && !tailscaleLocalIdentityPresent(stateDir) {
			return ProfileApplyResult{}, fmt.Errorf("an auth key is required before the first Tailscale connection")
		}
	}
	if err := config.Validate(cfg); err != nil {
		rollbackKey()
		return ProfileApplyResult{}, err
	}
	if err := deps.validate(cfg); err != nil {
		rollbackKey()
		return ProfileApplyResult{}, err
	}
	if err := writeAtomic(configPath, []byte(config.Render(cfg)), 0o640); err != nil {
		rollbackKey()
		return ProfileApplyResult{}, err
	}
	result := ProfileApplyResult{Revision: fileDigest(configPath)}
	if !wasRunning {
		return result, nil
	}
	if err := deps.reload(ctx, cfg); err != nil {
		rollbackErr := writeAtomic(configPath, original, 0o640)
		rollbackKey()
		var restartErr error
		if rollbackErr == nil {
			stillRunning, stateErr := deps.stateExists(previousCfg)
			if stateErr != nil {
				rollbackErr = fmt.Errorf("inspect gateway after failed reload: %w", stateErr)
			} else if !stillRunning {
				restartErr = deps.start(ctx, previousCfg)
			}
		}
		return ProfileApplyResult{}, tailscaleApplyRollbackError(err, rollbackErr, restartErr)
	}
	result.Reloaded = true
	return result, nil
}

func normalizeTailscaleUpdate(input *TailscaleUpdateRequest) error {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.DisplayName == "" {
		input.DisplayName = "Tailnet"
	}
	input.Hostname = strings.ToLower(strings.TrimSpace(input.Hostname))
	if input.Hostname == "" {
		input.Hostname = "opensurge-mac"
	}
	input.ControlURL = strings.TrimRight(strings.TrimSpace(input.ControlURL), "/")
	if input.ControlURL == "" {
		input.ControlURL = "https://controlplane.tailscale.com"
	}
	input.AuthKey = strings.TrimSpace(input.AuthKey)
	if strings.ContainsAny(input.AuthKey, " \t\r\n") {
		return fmt.Errorf("tailscale auth key must not contain whitespace")
	}
	input.MagicDNSSuffixes = normalizeStringList(input.MagicDNSSuffixes, func(value string) string {
		return strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "*."), ".")
	})
	var err error
	if input.PeerCIDRs, err = normalizeCIDRs(input.PeerCIDRs); err != nil {
		return fmt.Errorf("tailscale peer targets: %w", err)
	}
	if input.SubnetRoutes, err = normalizeCIDRs(input.SubnetRoutes); err != nil {
		return fmt.Errorf("tailscale subnet routes: %w", err)
	}
	input.AllowedDevices = normalizeStringList(input.AllowedDevices, strings.TrimSpace)
	if input.AllowAllDevices {
		input.AllowedDevices = nil
	}
	if len(input.SubnetRoutes) > 0 {
		input.AcceptRoutes = true
	}
	input.ExitNode = strings.TrimSpace(input.ExitNode)
	if input.ExitNode == "" {
		input.ExitNodeAllowLANAccess = false
	}
	return nil
}

func normalizeCIDRs(values []string) ([]string, error) {
	return normalizeStringListWithError(values, func(value string) (string, error) {
		value = strings.TrimSpace(value)
		if address, err := netip.ParseAddr(value); err == nil {
			return netip.PrefixFrom(address, address.BitLen()).String(), nil
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return "", fmt.Errorf("%q is not an IP address or CIDR", value)
		}
		return prefix.Masked().String(), nil
	})
}

func normalizeStringList(values []string, normalize func(string) string) []string {
	out, _ := normalizeStringListWithError(values, func(value string) (string, error) { return normalize(value), nil })
	return out
}

func normalizeStringListWithError(values []string, normalize func(string) (string, error)) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized, err := normalize(value)
		if err != nil {
			return nil, err
		}
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	return out, nil
}

func tailscaleManagedPaths(configPath string) (string, string) {
	dataDir := filepath.Join(filepath.Dir(configPath), "data")
	return filepath.Join(dataDir, "tailscale-auth-key"), filepath.Join(dataDir, "tailscale")
}

func tailscaleLocalIdentityPresent(stateDir string) bool {
	entries, err := os.ReadDir(stateDir)
	return err == nil && len(entries) > 0
}

type fileSnapshot struct {
	exists bool
	data   []byte
	mode   os.FileMode
}

func snapshotFile(path string) (fileSnapshot, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fileSnapshot{}, nil
	}
	if err != nil {
		return fileSnapshot{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	return fileSnapshot{exists: true, data: data, mode: info.Mode().Perm()}, nil
}

func restoreFile(path string, snapshot fileSnapshot) error {
	if !snapshot.exists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writeAtomic(path, snapshot.data, snapshot.mode)
}

func tailscaleApplyRollbackError(reloadErr, rollbackErr, restartErr error) error {
	message := fmt.Sprintf("apply Tailscale reload failed: %v", reloadErr)
	if rollbackErr != nil {
		message += fmt.Sprintf("; restore previous config failed: %v", rollbackErr)
	} else {
		message += "; previous config and auth key restored"
	}
	if restartErr != nil {
		message += fmt.Sprintf("; restart previous gateway failed: %v", restartErr)
	} else if rollbackErr == nil {
		message += "; previous running gateway preserved or restored"
	}
	return fmt.Errorf("%s", message)
}

func (DirectRunner) ForgetTailscaleIdentity(_ context.Context, configPath, revision string) (string, error) {
	return forgetTailscaleIdentity(configPath, revision, forgetTailscaleDeps{
		geteuid: os.Geteuid,
		stateExists: func(cfg config.Config) (bool, error) {
			_, exists, err := runtime.LoadState(runtime.NewPaths(cfg).StateFile)
			return exists, err
		},
		removeAll: os.RemoveAll,
	})
}

type forgetTailscaleDeps struct {
	geteuid     func() int
	stateExists func(config.Config) (bool, error)
	removeAll   func(string) error
}

func forgetTailscaleIdentity(configPath, revision string, deps forgetTailscaleDeps) (string, error) {
	if deps.geteuid() != 0 {
		return "", fmt.Errorf("privileged helper is required")
	}
	if revision == "" || revision != fileDigest(configPath) {
		return "", fmt.Errorf("config revision conflict")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return "", err
	}
	if cfg.Tailscale.Enabled {
		return "", fmt.Errorf("disable Tailscale before forgetting its local identity")
	}
	if running, err := deps.stateExists(cfg); err != nil {
		return "", err
	} else if running {
		return "", fmt.Errorf("gateway must be stopped before forgetting the Tailscale identity")
	}
	_, expectedStateDir := tailscaleManagedPaths(configPath)
	if filepath.Clean(cfg.Tailscale.StateDir) != filepath.Clean(expectedStateDir) {
		return "", fmt.Errorf("tailscale.state_dir is not managed by OpenSurge")
	}
	if err := deps.removeAll(expectedStateDir); err != nil {
		return "", err
	}
	return fileDigest(configPath), nil
}

func profileApplyRollbackError(reloadErr, rollbackErr, restartErr error) error {
	message := fmt.Sprintf("apply profile reload failed: %v", reloadErr)
	if rollbackErr != nil {
		message += fmt.Sprintf("; restore previous config failed: %v", rollbackErr)
	} else {
		message += "; previous config restored"
	}
	if restartErr != nil {
		message += fmt.Sprintf("; restart previous gateway failed: %v", restartErr)
	} else if rollbackErr == nil {
		message += "; previous running gateway preserved or restored"
	}
	return fmt.Errorf("%s", message)
}

func (DirectRunner) ApplyDevicePolicy(_ context.Context, configPath, revision string, payload []byte) (string, error) {
	if os.Geteuid() != 0 {
		return "", fmt.Errorf("privileged helper is required")
	}
	if len(payload) == 0 || len(payload) > maxSourceSize {
		return "", fmt.Errorf("device policy payload must be between 1 byte and 10 MiB")
	}
	cfg, err := config.LoadRuntime(configPath)
	if err != nil {
		return "", err
	}
	if cfg.DevicePolicy.File == "" {
		return "", fmt.Errorf("device_policy.file is not configured")
	}
	current, err := device.LoadPolicyBundle(cfg.DevicePolicy.File)
	if err != nil {
		return "", err
	}
	if revision == "" || revision != current.Digest {
		return "", fmt.Errorf("device policy revision conflict")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var policy device.PolicySet
	if err := decoder.Decode(&policy); err != nil {
		return "", err
	}
	if err := config.ValidateDevicePolicyCandidate(cfg, policy); err != nil {
		return "", err
	}
	scope, err := cfg.LANScope()
	if err != nil {
		return "", err
	}
	bundle, err := device.CompilePolicyBundleForLAN(policy, scope, cfg.Gateway.Mode == config.GatewayModeSameLAN)
	if err != nil {
		return "", err
	}
	formatted, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return "", err
	}
	temp, err := os.MkdirTemp(cfg.Runtime.Dir, ".device-policy-validation-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)
	validationPolicy := filepath.Join(temp, "device-policy.json")
	if err := os.WriteFile(validationPolicy, append(formatted, '\n'), 0o640); err != nil {
		return "", err
	}
	validation := cfg
	validation.DevicePolicy.File = validationPolicy
	validation.DevicePolicy.Bundle = &bundle
	validation.Runtime.Dir = temp
	validation.Mihomo.Config = filepath.Join(temp, "mihomo.yaml")
	if err := mihomo.New(validation, runtime.NewPaths(validation)).ValidateConfig(); err != nil {
		return "", err
	}
	if err := writeAtomic(cfg.DevicePolicy.File, append(formatted, '\n'), 0o640); err != nil {
		return "", err
	}
	return bundle.Digest, nil
}

func (DirectRunner) ApplyControlConfig(_ context.Context, configPath, revision string, payload []byte) (string, error) {
	if os.Geteuid() != 0 {
		return "", fmt.Errorf("privileged helper is required")
	}
	return applyControlConfig(configPath, revision, payload)
}

func applyControlConfig(configPath, revision string, payload []byte) (string, error) {
	if revision == "" || revision != fileDigest(configPath) {
		return "", fmt.Errorf("config revision conflict")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var input ControlConfig
	if err := decoder.Decode(&input); err != nil {
		return "", err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return "", err
	}
	paths := runtime.NewPaths(cfg)
	if _, exists, err := runtime.LoadState(paths.StateFile); err != nil {
		return "", err
	} else if exists {
		return "", fmt.Errorf("gateway must be stopped before editing network configuration")
	}
	cfg.Gateway.Mode = input.Gateway.Mode
	cfg.Gateway.Interface = input.Gateway.Interface
	cfg.Gateway.LANIP = input.Gateway.LANIP
	// Clients predating gateway.lan_prefix_len omit the field; the zero value
	// keeps their historical /24 behavior instead of failing the save.
	cfg.Gateway.LANPrefixLen = lan.PrefixLenOrDefault(input.Gateway.LANPrefixLen)
	cfg.Gateway.UpstreamInterface = input.Gateway.UpstreamInterface
	cfg.DHCP.Enabled = input.DHCP.Enabled
	cfg.DHCP.RangeStart = input.DHCP.RangeStart
	cfg.DHCP.RangeEnd = input.DHCP.RangeEnd
	cfg.DHCP.LeaseTime = input.DHCP.LeaseTime
	cfg.DHCP.Domain = input.DHCP.Domain
	cfg.DHCP.BypassGateway = input.DHCP.BypassGateway
	cfg.DHCP.BypassDNS = append([]string(nil), input.DHCP.BypassDNS...)
	cfg.DNS.Listen = input.DNS.Listen
	cfg.DNS.Upstream = input.DNS.Upstream
	cfg.DNS.IPv6 = input.DNS.IPv6
	if input.Mihomo.StoreFakeIP != nil {
		cfg.Mihomo.StoreFakeIP = *input.Mihomo.StoreFakeIP
	}
	cfg.Transparent.Mode = input.Transparent.Mode
	cfg.Transparent.TUNStrictRoute = input.Transparent.StrictRoute
	cfg.Transparent.TUNIPv6 = input.Transparent.TUNIPv6
	cfg.Transparent.IPv6SharedL2Ready = input.Transparent.IPv6SharedL2Ready
	if cfg.Transparent.TUNIPv6 == "" {
		// Schema v1 clients predating the IPv6 control omit this field. Treat
		// omission as the fail-safe default instead of making an otherwise valid
		// legacy save fail validation.
		cfg.Transparent.TUNIPv6 = config.TUNIPv6Off
	}
	cfg.LocalSystemProxy.Enabled = input.LocalSystemProxy.Enabled
	cfg.DevicePolicy.ProtectedIPv4 = append([]string(nil), input.DevicePolicy.ProtectedIPv4...)
	createdPolicy := ""
	if input.DevicePolicy.Enabled && cfg.DevicePolicy.File == "" {
		createdPolicy = filepath.Join(filepath.Dir(configPath), "data", "device-policy.json")
		empty := []byte("{\n  \"devices\": [],\n  \"profiles\": [],\n  \"templates\": [],\n  \"rule_sets\": []\n}\n")
		if err := writeAtomic(createdPolicy, empty, 0o640); err != nil {
			return "", err
		}
		cfg.DevicePolicy.File = createdPolicy
	} else if !input.DevicePolicy.Enabled {
		cfg.DevicePolicy.File = ""
		cfg.DevicePolicy.ProtectedIPv4 = nil
	}
	cfg.DevicePolicy.Bundle = nil
	if err := config.Validate(cfg); err != nil {
		if createdPolicy != "" {
			_ = os.Remove(createdPolicy)
		}
		return "", err
	}
	if err := writeAtomic(configPath, []byte(config.Render(cfg)), 0o640); err != nil {
		if createdPolicy != "" {
			_ = os.Remove(createdPolicy)
		}
		return "", err
	}
	return fileDigest(configPath), nil
}
