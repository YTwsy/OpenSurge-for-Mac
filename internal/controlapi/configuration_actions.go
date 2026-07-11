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
	if err := writeAtomic(cfg.DevicePolicy.File, append(formatted, '\n'), 0o640); err != nil {
		return "", err
	}
	return bundle.Digest, nil
}
