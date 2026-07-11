package controlapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/mihomo"
	"open-mihomo-gateway/internal/runtime"
)

func (DirectRunner) ApplyProfile(_ context.Context, configPath, revision string, payload []byte) (string, error) {
	if os.Geteuid() != 0 {
		return "", fmt.Errorf("privileged helper is required")
	}
	if len(payload) == 0 || len(payload) > maxSourceSize {
		return "", fmt.Errorf("profile payload must be between 1 byte and 10 MiB")
	}
	if revision == "" || revision != fileDigest(configPath) {
		return "", fmt.Errorf("config revision conflict")
	}
	if _, err := inspectSource(payload, "mihomo_profile"); err != nil {
		return "", err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	profilePath := filepath.Join(filepath.Dir(configPath), "data", "imported-profile-"+hex.EncodeToString(digest[:8])+".yaml")
	if err := writeAtomic(profilePath, payload, 0o640); err != nil {
		return "", err
	}
	cfg.Mihomo.ProfileMode = config.MihomoProfileModeImported
	cfg.Mihomo.Profile = profilePath
	temp, err := os.MkdirTemp(cfg.Runtime.Dir, ".profile-validation-*")
	if err != nil {
		_ = os.Remove(profilePath)
		return "", err
	}
	defer os.RemoveAll(temp)
	validation := cfg
	validation.Runtime.Dir = temp
	validation.Mihomo.Config = filepath.Join(temp, "mihomo.yaml")
	if err := mihomo.New(validation, runtime.NewPaths(validation)).ValidateConfig(); err != nil {
		_ = os.Remove(profilePath)
		return "", err
	}
	if err := writeAtomic(configPath, []byte(config.Render(cfg)), 0o640); err != nil {
		_ = os.Remove(profilePath)
		return "", err
	}
	return fileDigest(configPath), nil
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
	if err := device.ValidatePolicySetForLANWithProtected(policy, cfg.Gateway.LANIP, cfg.DevicePolicy.ProtectedIPv4); err != nil {
		return "", err
	}
	bundle, err := device.CompilePolicyBundle(policy)
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
	validation.DevicePolicy.Bundle = nil
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
	cfg.Gateway.UpstreamInterface = input.Gateway.UpstreamInterface
	cfg.DHCP.Enabled = input.DHCP.Enabled
	cfg.DHCP.RangeStart = input.DHCP.RangeStart
	cfg.DHCP.RangeEnd = input.DHCP.RangeEnd
	cfg.DHCP.LeaseTime = input.DHCP.LeaseTime
	cfg.DHCP.Domain = input.DHCP.Domain
	cfg.DNS.Listen = input.DNS.Listen
	cfg.DNS.Upstream = input.DNS.Upstream
	cfg.Transparent.Mode = input.Transparent.Mode
	cfg.Transparent.TUNStrictRoute = input.Transparent.StrictRoute
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
