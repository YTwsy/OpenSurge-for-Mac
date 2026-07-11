package device

import (
	"os"
	"path/filepath"
	"testing"
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
