package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

// MihomoProfileDigest identifies the imported profile selected by the desired
// gateway configuration. Managed profiles have no external source digest.
func MihomoProfileDigest(cfg Config) (string, error) {
	if cfg.Mihomo.ProfileMode != MihomoProfileModeImported || cfg.Mihomo.Profile == "" {
		return "", nil
	}
	data, err := os.ReadFile(cfg.Mihomo.Profile)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
