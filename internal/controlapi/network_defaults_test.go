package controlapi

import (
	"testing"

	"open-mihomo-gateway/internal/macosnetwork"
)

func TestSuggestDHCPRangeUsesPreferredPool(t *testing.T) {
	snapshot := macosnetwork.Snapshot{IPv4: "192.168.1.20", SubnetMask: "255.255.255.0", Router: "192.168.1.1"}
	start, end, err := suggestDHCPRange(snapshot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if start != "192.168.1.100" || end != "192.168.1.200" {
		t.Fatalf("range = %s-%s", start, end)
	}
}

func TestSuggestDHCPRangeExcludesGatewayAndProtectedAddresses(t *testing.T) {
	snapshot := macosnetwork.Snapshot{IPv4: "192.168.1.190", SubnetMask: "255.255.255.0", Router: "192.168.1.1"}
	start, end, err := suggestDHCPRange(snapshot, []string{"192.168.1.180"})
	if err != nil {
		t.Fatal(err)
	}
	if start != "192.168.1.100" || end != "192.168.1.179" {
		t.Fatalf("range = %s-%s", start, end)
	}
}

func TestSuggestDHCPRangeSupportsWiderSubnets(t *testing.T) {
	snapshot := macosnetwork.Snapshot{IPv4: "192.168.1.20", SubnetMask: "255.255.254.0", Router: "192.168.1.1"}
	start, end, err := suggestDHCPRange(snapshot, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 192.168.1.20/23 lives in 192.168.0.0/23, so subnet offsets 100-200 land
	// in the first half of the range rather than the third octet the Mac is on.
	if start != "192.168.0.100" || end != "192.168.0.200" {
		t.Fatalf("range = %s-%s", start, end)
	}
}

func TestSuggestDHCPRangeFitsSmallSubnets(t *testing.T) {
	snapshot := macosnetwork.Snapshot{IPv4: "192.168.1.20", SubnetMask: "255.255.255.240", Router: "192.168.1.17"}
	start, end, err := suggestDHCPRange(snapshot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if start != "192.168.1.21" || end != "192.168.1.30" {
		t.Fatalf("range = %s-%s", start, end)
	}
}

func TestSnapshotPrefixLenRejectsMissingMask(t *testing.T) {
	if _, err := snapshotPrefixLen(macosnetwork.Snapshot{IPv4: "192.168.1.20"}); err == nil {
		t.Fatal("a snapshot without a subnet mask should not produce a prefix length")
	}
}
