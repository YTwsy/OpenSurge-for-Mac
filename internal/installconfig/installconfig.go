package installconfig

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"open-mihomo-gateway/internal/config"
)

// Prepare creates the root-owned applied configuration tree used by the
// privileged helper. Executables are installed separately by the installer.
func Prepare(sourcePath, root string) (config.Config, error) {
	cfg, err := config.Load(sourcePath)
	if err != nil {
		return config.Config{}, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return config.Config{}, err
	}
	for _, dir := range []string{root, filepath.Join(root, "bin"), filepath.Join(root, "data"), filepath.Join(root, "runtime")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return config.Config{}, err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return config.Config{}, err
		}
	}
	cfg.DHCP.Binary = filepath.Join(root, "bin", "dnsmasq")
	cfg.Mihomo.Binary = filepath.Join(root, "bin", "mihomo")
	cfg.Mihomo.Config = filepath.Join(root, "runtime", "mihomo.yaml")
	cfg.Runtime.Dir = filepath.Join(root, "runtime")
	if cfg.Mihomo.ProfileMode == config.MihomoProfileModeImported {
		destination := filepath.Join(root, "data", "imported-profile.yaml")
		if err := copyPrivateFile(cfg.Mihomo.Profile, destination); err != nil {
			return config.Config{}, fmt.Errorf("copy imported profile: %w", err)
		}
		cfg.Mihomo.Profile = destination
	}
	if cfg.DevicePolicy.File != "" {
		destination := filepath.Join(root, "data", "device-policy.json")
		if err := copyPrivateFile(cfg.DevicePolicy.File, destination); err != nil {
			return config.Config{}, fmt.Errorf("copy device policy: %w", err)
		}
		cfg.DevicePolicy.File = destination
		cfg.DevicePolicy.Bundle = nil
	}
	if err := config.Validate(cfg); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func ValidatePackageSource(sourcePath string) error {
	cfg, err := config.Load(sourcePath)
	if err != nil {
		return err
	}
	if cfg.Mihomo.ProfileMode != config.MihomoProfileModeManaged || cfg.Mihomo.Profile != "" {
		return fmt.Errorf("installer seed config must use a managed profile; import profiles after installation")
	}
	if cfg.DevicePolicy.File != "" {
		return fmt.Errorf("installer seed config must not reference a device policy file; configure it after installation")
	}
	return nil
}

func Write(cfg config.Config, destination string) error {
	return writePrivateFile(destination, []byte(config.Render(cfg)))
}

func copyPrivateFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".opensurge-install-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, input); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, destination)
}

func writePrivateFile(destination string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".opensurge-install-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, destination)
}
