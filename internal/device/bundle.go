package device

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// PolicyBundle is the immutable policy artifact consumed by one gateway run.
// CanonicalJSON and Digest identify the exact declarative source, while
// Compiled contains the shared DHCP and mihomo representation derived from it.
type PolicyBundle struct {
	SchemaVersion int             `json:"schema_version"`
	Digest        string          `json:"digest"`
	CanonicalJSON json.RawMessage `json:"canonical_json"`
	Policy        PolicySet       `json:"policy"`
	Compiled      CompiledPolicy  `json:"compiled"`
}

func LoadPolicyBundle(path string) (PolicyBundle, error) {
	set, err := LoadPolicySet(path)
	if err != nil {
		return PolicyBundle{}, err
	}
	return CompilePolicyBundle(set)
}

func CompilePolicyBundle(set PolicySet) (PolicyBundle, error) {
	if err := ValidatePolicySet(set); err != nil {
		return PolicyBundle{}, err
	}
	canonical, err := json.Marshal(set)
	if err != nil {
		return PolicyBundle{}, fmt.Errorf("canonicalize device policy: %w", err)
	}
	compiled, err := CompilePolicySet(set)
	if err != nil {
		return PolicyBundle{}, err
	}
	digest := sha256.Sum256(canonical)
	return PolicyBundle{
		SchemaVersion: 1,
		Digest:        hex.EncodeToString(digest[:]),
		CanonicalJSON: append(json.RawMessage(nil), canonical...),
		Policy:        set,
		Compiled:      compiled,
	}, nil
}

func LoadPolicyBundleSnapshot(path string) (PolicyBundle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PolicyBundle{}, err
	}
	var bundle PolicyBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return PolicyBundle{}, fmt.Errorf("decode applied device policy bundle: %w", err)
	}
	if bundle.SchemaVersion != 1 {
		return PolicyBundle{}, fmt.Errorf("unsupported applied device policy bundle schema %d", bundle.SchemaVersion)
	}
	if err := ValidatePolicySet(bundle.Policy); err != nil {
		return PolicyBundle{}, fmt.Errorf("validate applied device policy bundle: %w", err)
	}
	canonical, err := json.Marshal(bundle.Policy)
	if err != nil {
		return PolicyBundle{}, fmt.Errorf("canonicalize applied device policy bundle: %w", err)
	}
	digest := sha256.Sum256(canonical)
	var compact bytes.Buffer
	if err := json.Compact(&compact, bundle.CanonicalJSON); err != nil {
		return PolicyBundle{}, fmt.Errorf("compact applied device policy canonical JSON: %w", err)
	}
	if compact.String() != string(canonical) {
		return PolicyBundle{}, fmt.Errorf("applied device policy bundle canonical JSON mismatch")
	}
	if bundle.Digest != hex.EncodeToString(digest[:]) {
		return PolicyBundle{}, fmt.Errorf("applied device policy bundle digest mismatch")
	}
	compiled, err := CompilePolicySet(bundle.Policy)
	if err != nil {
		return PolicyBundle{}, fmt.Errorf("compile applied device policy bundle: %w", err)
	}
	bundle.Compiled = compiled
	bundle.CanonicalJSON = append(json.RawMessage(nil), compact.Bytes()...)
	return bundle, nil
}

func WritePolicyBundleSnapshot(path string, bundle PolicyBundle) error {
	if bundle.SchemaVersion != 1 || bundle.Digest == "" {
		return fmt.Errorf("invalid device policy bundle")
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".device-policy-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func RemovePolicyBundleSnapshot(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
