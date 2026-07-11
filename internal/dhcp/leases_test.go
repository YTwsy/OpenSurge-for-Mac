package dhcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"open-mihomo-gateway/internal/device"
)

func TestReconcilePolicyLeasesRemovesOnlyStaleManagedLease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dnsmasq.leases")
	leases := "2000000000 aa:bb:cc:dd:ee:01 192.168.50.141 stale 01:aa\n" +
		"2000000000 aa:bb:cc:dd:ee:02 192.168.50.102 keep 01:bb\n"
	if err := os.WriteFile(path, []byte(leases), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ReconcilePolicyLeases(path, []device.Reservation{{ID: "phone", MAC: "aa:bb:cc:dd:ee:01", IPv4: "192.168.50.101"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "192.168.50.141") || !strings.Contains(string(data), "192.168.50.102") {
		t.Fatalf("leases after reconcile:\n%s", data)
	}
}
