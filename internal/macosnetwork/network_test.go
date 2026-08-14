package macosnetwork

import (
	"context"
	"net"
	"testing"
)

func TestParseNetworkInfo(t *testing.T) {
	got := parseNetworkInfo("DHCP Configuration\nIP address: 192.168.1.20\nSubnet mask: 255.255.255.0\nRouter: 192.168.1.1\n")
	if got.IPv4Mode != IPv4ModeDHCP || got.IPv4 != "192.168.1.20" || got.SubnetMask != "255.255.255.0" || got.Router != "192.168.1.1" {
		t.Fatalf("parseNetworkInfo() = %#v", got)
	}
	manual := parseNetworkInfo("Manual Configuration\nIP address: 192.168.1.21\nSubnet mask: 255.255.255.0\nRouter: 192.168.1.1\n")
	if manual.IPv4Mode != IPv4ModeManual {
		t.Fatalf("manual IPv4 mode = %q", manual.IPv4Mode)
	}
}

func TestParseDNSAndIPv6Default(t *testing.T) {
	dns := parseDNS("192.168.1.20\n1.1.1.1\n")
	if len(dns) != 2 {
		t.Fatalf("parseDNS() = %#v", dns)
	}
	routes := "default fe80::1%en0 UGcg en0\n::1 ::1 UHL lo0\n"
	if !hasIPv6DefaultRoute(routes, "en0") {
		t.Fatal("IPv6 default route not detected")
	}
	if hasIPv6DefaultRoute(routes, "en7") {
		t.Fatal("IPv6 default route detected on wrong interface")
	}
}

func TestIPv6DefaultRouteStateDistinguishesSelfAndCompetingGateways(t *testing.T) {
	localAddresses := []net.Addr{
		stringAddr("fe80::1851:c102:7eba:c3a9%en0/64"),
		stringAddr("fdfe:dcba:9878::1/64"),
	}

	tests := []struct {
		name     string
		routes   string
		active   bool
		selfOnly bool
	}{
		{
			name:     "OpenSurge self route",
			routes:   "default fe80::1851:c102:7eba:c3a9%en0 UGcg en0\n",
			active:   true,
			selfOnly: true,
		},
		{
			name:     "external router",
			routes:   "default fe80::1%en0 UGcg en0 1200\n",
			active:   true,
			selfOnly: false,
		},
		{
			name: "self and external routers",
			routes: "default fe80::1851:c102:7eba:c3a9%en0 UGcg en0\n" +
				"default fe80::1%en0 UGcg en0\n",
			active:   true,
			selfOnly: false,
		},
		{
			name:     "other interface",
			routes:   "default fe80::1%en7 UGcg en7\n",
			active:   false,
			selfOnly: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			active, selfOnly := ipv6DefaultRouteState(test.routes, "en0", localAddresses)
			if active != test.active || selfOnly != test.selfOnly {
				t.Fatalf("route state = active %t, self-only %t", active, selfOnly)
			}
		})
	}
}

func TestSnapshotCompetingIPv6DefaultIsConservativeForLegacySnapshots(t *testing.T) {
	if (Snapshot{IPv6Default: true, IPv6DefaultSelfOnly: true}).CompetingIPv6Default() {
		t.Fatal("self-only IPv6 default route was treated as competing")
	}
	if !(Snapshot{IPv6Default: true}).CompetingIPv6Default() {
		t.Fatal("legacy IPv6 default route snapshot lost its conservative warning")
	}
}

func TestParseServiceInterface(t *testing.T) {
	output := `An asterisk (*) denotes that a network service is disabled.
(1) Wi-Fi
(Hardware Port: Wi-Fi, Device: en0)
(2) Thunderbolt Bridge
(Hardware Port: Thunderbolt Bridge, Device: bridge0)
`
	device, err := parseServiceInterface(output, "Wi-Fi")
	if err != nil {
		t.Fatal(err)
	}
	if device != "en0" {
		t.Fatalf("device = %q", device)
	}
	if _, err := parseServiceInterface(output, "Missing"); err == nil {
		t.Fatal("missing service should fail")
	}
	if got := parseServiceOrder(output)["Thunderbolt Bridge"]; got != "bridge0" {
		t.Fatalf("bridge service = %q", got)
	}
}

func TestInterfaceOptionsAreSortedAndNamed(t *testing.T) {
	options := interfaceOptions(map[string]string{
		"USB 10/100/1000 LAN": "en7",
		"Wi-Fi":               "en0",
		"":                    "en9",
	})
	if len(options) != 2 {
		t.Fatalf("options = %#v", options)
	}
	if options[0].Interface != "en0" || options[0].NetworkService != "Wi-Fi" {
		t.Fatalf("first option = %#v", options[0])
	}
	if options[1].Interface != "en7" || options[1].NetworkService != "USB 10/100/1000 LAN" {
		t.Fatalf("second option = %#v", options[1])
	}
}

func TestFirstLinkLocalIPv6IgnoresOtherAddressesAndScope(t *testing.T) {
	addrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("192.168.1.20"), Mask: net.CIDRMask(24, 32)},
		stringAddr("2001:db8::20/64"),
		stringAddr("fe80::abcd%en0/64"),
	}
	if got := firstLinkLocalIPv6(addrs); got != "fe80::abcd" {
		t.Fatalf("link-local IPv6 = %q", got)
	}
}

type stringAddr string

func (address stringAddr) Network() string { return "ip" }
func (address stringAddr) String() string  { return string(address) }

func TestValidateManual(t *testing.T) {
	valid := ManualConfig{NetworkService: "Wi-Fi", Interface: "en0", IPv4: "192.168.1.20", SubnetMask: "255.255.255.0", Router: "192.168.1.1", DNS: []string{"1.1.1.1"}}
	if err := ValidateManual(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Router = "192.168.2.1"
	if err := ValidateManual(invalid); err == nil {
		t.Fatal("router outside subnet should fail")
	}
}

func TestVerifyManualRequiresManualModeAndExpectedIPv4(t *testing.T) {
	expected := ManualConfig{NetworkService: "Wi-Fi", Interface: "en0", IPv4: "192.168.1.20", SubnetMask: "255.255.255.0", Router: "192.168.1.1"}
	applied := Snapshot{NetworkService: "Wi-Fi", Interface: "en0", IPv4Mode: IPv4ModeManual, IPv4: expected.IPv4, SubnetMask: expected.SubnetMask, Router: expected.Router}
	if err := VerifyManual(applied, expected); err != nil {
		t.Fatal(err)
	}
	applied.IPv4Mode = IPv4ModeDHCP
	if err := VerifyManual(applied, expected); err == nil {
		t.Fatal("DHCP configuration should not verify as fixed IPv4")
	}
	applied.IPv4Mode = IPv4ModeManual
	applied.IPv4 = "192.168.1.99"
	if err := VerifyManual(applied, expected); err == nil {
		t.Fatal("unexpected manual IPv4 should not verify")
	}
}

func TestLookupRouteReturnsSelectedInterface(t *testing.T) {
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(_ context.Context, binary string, args ...string) (string, error) {
		if binary != "/sbin/route" || len(args) != 3 {
			t.Fatalf("command = %s %#v", binary, args)
		}
		return "   gateway: 198.18.0.1\n interface: utun42\n", nil
	}

	route, err := LookupRoute(t.Context(), "1.0.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if route.Interface != "utun42" || route.Gateway != "198.18.0.1" {
		t.Fatalf("route = %#v", route)
	}
}

func TestDiscoverDefaultUsesDefaultRouteNetworkService(t *testing.T) {
	original := runCommand
	t.Cleanup(func() { runCommand = original })
	runCommand = func(_ context.Context, binary string, args ...string) (string, error) {
		switch {
		case binary == "/sbin/route":
			return "   gateway: 192.168.1.1\n interface: en7\n", nil
		case binary == "/usr/sbin/networksetup" && len(args) == 1 && args[0] == "-listnetworkserviceorder":
			return "(1) Wi-Fi\n(Hardware Port: Wi-Fi, Device: en0)\n(2) USB LAN\n(Hardware Port: USB LAN, Device: en7)\n", nil
		case binary == "/usr/sbin/networksetup" && len(args) == 2 && args[0] == "-getinfo" && args[1] == "USB LAN":
			return "DHCP Configuration\nIP address: 192.168.1.190\nSubnet mask: 255.255.255.0\nRouter: 192.168.1.1\n", nil
		case binary == "/usr/sbin/networksetup" && len(args) == 2 && args[0] == "-getdnsservers" && args[1] == "USB LAN":
			return "192.168.1.1\n", nil
		case binary == "/usr/sbin/netstat":
			return "", nil
		default:
			t.Fatalf("unexpected command: %s %#v", binary, args)
			return "", nil
		}
	}

	snapshot, err := DiscoverDefault(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.NetworkService != "USB LAN" || snapshot.Interface != "en7" || snapshot.IPv4 != "192.168.1.190" || snapshot.SubnetMask != "255.255.255.0" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestParseRouteGet(t *testing.T) {
	got := parseRouteGet("   route to: 1.1.1.1\n    gateway: 198.18.0.1\n  interface: utun123\n")
	if got.Interface != "utun123" || got.Gateway != "198.18.0.1" {
		t.Fatalf("parseRouteGet() = %#v", got)
	}
}
