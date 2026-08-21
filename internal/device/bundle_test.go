package device

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/lan"
)

func TestPolicyBundleSnapshotRoundTripPreservesDigestAndCompiledPolicy(t *testing.T) {
	set := PolicySet{
		Profiles: []Profile{{ID: "home", DefaultPolicies: []string{"DIRECT"}}},
		Devices:  []ManagedDevice{{ID: "phone", MAC: "aa:bb:cc:dd:ee:01", IPv4: "192.168.50.101", Profile: "home"}},
	}
	bundle, err := CompilePolicyBundle(set)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "device-policy.applied.json")
	if err := WritePolicyBundleSnapshot(path, bundle); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPolicyBundleSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != bundle.Digest || len(loaded.Compiled.Reservations) != 1 || loaded.Compiled.Reservations[0].IPv4 != "192.168.50.101" {
		t.Fatalf("loaded bundle = %#v", loaded)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"digest":"wrong"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicyBundleSnapshot(path); err == nil {
		t.Fatal("LoadPolicyBundleSnapshot() accepted a corrupted snapshot")
	}
}

func TestPolicyBundleSnapshotPreservesPausedIPOnlyCompilation(t *testing.T) {
	set := PolicySet{
		Profiles: []Profile{{ID: "home", DefaultPolicies: []string{"DIRECT"}}},
		Devices:  []ManagedDevice{{ID: "phone", IPv4: "192.168.50.101", Profile: "home"}},
	}
	bundle, err := CompilePolicyBundleForIPOnlyMode(set, false)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "device-policy.applied.json")
	if err := WritePolicyBundleSnapshot(path, bundle); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPolicyBundleSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.IPOnlyDevicesActive || len(loaded.Compiled.Devices) != 0 || len(loaded.Policy.Devices) != 1 {
		t.Fatalf("loaded paused bundle = %#v", loaded)
	}
}

func TestPolicyBundleForLANKeepsDesiredPolicyButDormantDevicesOutOfRuntime(t *testing.T) {
	set := PolicySet{
		Profiles: []Profile{{
			ID:              "home",
			DefaultPolicies: []string{"DIRECT"},
			Rules:           []Rule{{ID: "media", Match: RuleMatch{Domains: []string{"example.com"}}, Policies: []string{"DIRECT"}}},
		}},
		Devices: []ManagedDevice{
			{ID: "active", MAC: "aa:bb:cc:dd:ee:01", IPv4: "192.168.51.101", Profile: "home", EgressMode: EgressModeDedicated},
			{ID: "dormant", MAC: "aa:bb:cc:dd:ee:02", IPv4: "192.168.60.101", Profile: "home", EgressMode: EgressModeDedicated},
		},
	}
	scope, err := lan.NewScope("192.168.50.1", 22)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := CompilePolicyBundleForLAN(set, scope, false)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.ActiveLAN != "192.168.48.0/22" || len(bundle.Policy.Devices) != 2 {
		t.Fatalf("desired bundle = %#v", bundle)
	}
	if len(bundle.Compiled.Devices) != 1 || bundle.Compiled.Devices[0].ID != "active" || len(bundle.Compiled.Reservations) != 1 {
		t.Fatalf("compiled bundle = %#v", bundle.Compiled)
	}
	compiledJSON, err := json.Marshal(bundle.Compiled)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(compiledJSON), "dormant") || strings.Contains(string(compiledJSON), "192.168.60.101") {
		t.Fatalf("dormant device leaked into compiled policy: %s", compiledJSON)
	}

	path := filepath.Join(t.TempDir(), "device-policy.applied.json")
	if err := WritePolicyBundleSnapshot(path, bundle); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPolicyBundleSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != bundle.Digest || loaded.ActiveLAN != scope.String() || len(loaded.Policy.Devices) != 2 || len(loaded.Compiled.Devices) != 1 || loaded.Compiled.Devices[0].ID != "active" {
		t.Fatalf("loaded scoped bundle = %#v", loaded)
	}
	bundle.ActiveLAN = "::/0"
	if err := WritePolicyBundleSnapshot(path, bundle); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicyBundleSnapshot(path); err == nil || !strings.Contains(err.Error(), "invalid applied device policy active LAN") {
		t.Fatalf("LoadPolicyBundleSnapshot() error = %v", err)
	}
}
