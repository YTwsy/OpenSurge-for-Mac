package gateway

import (
	"strings"
	"testing"
)

func TestStatusFormatLabelsDNSOnlyMode(t *testing.T) {
	status := Status{
		Gateway:   "running",
		Interface: "en0",
		LANIP:     "192.168.1.20",
		DHCP:      "running",
	}

	got := status.Format()
	if !strings.Contains(got, "DNS: running") {
		t.Fatalf("status did not label DNS-only mode:\n%s", got)
	}
	if strings.Contains(got, "DHCP: running") {
		t.Fatalf("status incorrectly labeled DNS-only mode as DHCP:\n%s", got)
	}
}

func TestStatusFormatLabelsDHCPMode(t *testing.T) {
	status := Status{
		Gateway:     "running",
		Interface:   "en7",
		LANIP:       "192.168.50.1",
		DHCP:        "running",
		DHCPEnabled: true,
	}

	got := status.Format()
	if !strings.Contains(got, "DHCP: running") {
		t.Fatalf("status did not preserve DHCP label:\n%s", got)
	}
}

func TestStatusFormatIncludesIPv6RuntimeState(t *testing.T) {
	status := Status{
		Gateway:          "running",
		DNSIPv6:          true,
		TUNIPv6Requested: "auto",
		TUNIPv6Effective: true,
		IPv6Reason:       "native_ipv6_available",
	}

	got := status.Format()
	for _, want := range []string{
		"IPv6 DNS queries: true",
		"TUN IPv6: requested=auto effective=true (native_ipv6_available)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status missing %q:\n%s", want, got)
		}
	}
}
