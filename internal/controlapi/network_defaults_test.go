package controlapi

import (
	"testing"

	"open-mihomo-gateway/internal/macosnetwork"
)

func TestSuggestDHCPRange24UsesPreferredPool(t *testing.T) {
	snapshot := macosnetwork.Snapshot{IPv4: "192.168.1.20", SubnetMask: "255.255.255.0", Router: "192.168.1.1"}
	start, end, err := suggestDHCPRange24(snapshot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if start != "192.168.1.100" || end != "192.168.1.200" {
		t.Fatalf("range = %s-%s", start, end)
	}
}

func TestSuggestDHCPRange24ExcludesGatewayAndProtectedAddresses(t *testing.T) {
	snapshot := macosnetwork.Snapshot{IPv4: "192.168.1.190", SubnetMask: "255.255.255.0", Router: "192.168.1.1"}
	start, end, err := suggestDHCPRange24(snapshot, []string{"192.168.1.180"})
	if err != nil {
		t.Fatal(err)
	}
	if start != "192.168.1.100" || end != "192.168.1.179" {
		t.Fatalf("range = %s-%s", start, end)
	}
}

func TestSuggestDHCPRange24RejectsUnsupportedSubnet(t *testing.T) {
	snapshot := macosnetwork.Snapshot{IPv4: "192.168.1.20", SubnetMask: "255.255.254.0", Router: "192.168.1.1"}
	if _, _, err := suggestDHCPRange24(snapshot, nil); err == nil {
		t.Fatal("non-/24 subnet should not receive automatic DHCP defaults")
	}
}
