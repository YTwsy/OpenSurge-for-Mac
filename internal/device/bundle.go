package device

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"open-mihomo-gateway/internal/lan"
)

// PolicyBundle is the immutable policy artifact consumed by one gateway run.
// CanonicalJSON and Digest identify the exact declarative source, while
// Compiled contains the shared DHCP and mihomo representation derived from it.
type PolicyBundle struct {
	SchemaVersion       int             `json:"schema_version"`
	Digest              string          `json:"digest"`
	CanonicalJSON       json.RawMessage `json:"canonical_json"`
	Policy              PolicySet       `json:"policy"`
	Compiled            CompiledPolicy  `json:"compiled"`
	IPOnlyDevicesActive bool            `json:"ip_only_devices_active,omitempty"`
	ActiveLAN           string          `json:"active_lan,omitempty"`
}

func LoadPolicyBundle(path string) (PolicyBundle, error) {
	return LoadPolicyBundleForIPOnlyMode(path, true)
}

func LoadPolicyBundleForIPOnlyMode(path string, ipOnlyDevicesActive bool) (PolicyBundle, error) {
	set, err := LoadPolicySet(path)
	if err != nil {
		return PolicyBundle{}, err
	}
	return CompilePolicyBundleForIPOnlyMode(set, ipOnlyDevicesActive)
}

// LoadPolicyBundleForLAN keeps the complete policy as desired state but only
// compiles devices whose IPv4 belongs to scope into the runtime bundle.
func LoadPolicyBundleForLAN(path string, scope lan.Scope, ipOnlyDevicesActive bool) (PolicyBundle, error) {
	set, err := LoadPolicySet(path)
	if err != nil {
		return PolicyBundle{}, err
	}
	return CompilePolicyBundleForLAN(set, scope, ipOnlyDevicesActive)
}

func CompilePolicyBundle(set PolicySet) (PolicyBundle, error) {
	return CompilePolicyBundleForIPOnlyMode(set, true)
}

func CompilePolicyBundleForIPOnlyMode(set PolicySet, ipOnlyDevicesActive bool) (PolicyBundle, error) {
	return compilePolicyBundle(set, set, ipOnlyDevicesActive, "")
}

// CompilePolicyBundleForLAN records the full declarative policy and its digest
// while compiling only devices active on the configured LAN.
func CompilePolicyBundleForLAN(set PolicySet, scope lan.Scope, ipOnlyDevicesActive bool) (PolicyBundle, error) {
	return compilePolicyBundle(set, ActivePolicySetForLAN(set, scope), ipOnlyDevicesActive, scope.String())
}

func compilePolicyBundle(set, active PolicySet, ipOnlyDevicesActive bool, activeLAN string) (PolicyBundle, error) {
	if err := ValidatePolicySet(set); err != nil {
		return PolicyBundle{}, err
	}
	canonical, err := json.Marshal(set)
	if err != nil {
		return PolicyBundle{}, fmt.Errorf("canonicalize device policy: %w", err)
	}
	compiled, err := CompilePolicySetForIPOnlyMode(active, ipOnlyDevicesActive)
	if err != nil {
		return PolicyBundle{}, err
	}
	digest := sha256.Sum256(canonical)
	return PolicyBundle{
		SchemaVersion:       1,
		Digest:              hex.EncodeToString(digest[:]),
		CanonicalJSON:       append(json.RawMessage(nil), canonical...),
		Policy:              set,
		Compiled:            compiled,
		IPOnlyDevicesActive: ipOnlyDevicesActive,
		ActiveLAN:           activeLAN,
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
	active := bundle.Policy
	if bundle.ActiveLAN != "" {
		_, network, parseErr := net.ParseCIDR(bundle.ActiveLAN)
		prefixLen, bits := 0, 0
		if parseErr == nil {
			prefixLen, bits = network.Mask.Size()
		}
		if parseErr != nil || network.IP.To4() == nil || bits != 32 || !lan.ValidPrefixLen(prefixLen) || network.String() != bundle.ActiveLAN {
			return PolicyBundle{}, fmt.Errorf("invalid applied device policy active LAN %q", bundle.ActiveLAN)
		}
		active = activePolicySetForNetwork(bundle.Policy, network)
	}
	compiled, err := CompilePolicySetForIPOnlyMode(active, bundle.IPOnlyDevicesActive)
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
