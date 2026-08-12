package ipv6packet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBrokerBinaryFromRuntimeSiblingBin(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	broker := filepath.Join(binDir, "opensurge-network")
	if err := os.WriteFile(broker, []byte("broker"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())

	got, err := resolveBrokerBinary("opensurge-network", runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != broker {
		t.Fatalf("resolved broker = %q, want %q", got, broker)
	}
}

func TestResolveBrokerBinaryKeepsExplicitPath(t *testing.T) {
	broker := filepath.Join(t.TempDir(), "custom-network")
	if err := os.WriteFile(broker, []byte("broker"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := resolveBrokerBinary(broker, filepath.Join(t.TempDir(), "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	if got != broker {
		t.Fatalf("resolved broker = %q, want %q", got, broker)
	}
}
