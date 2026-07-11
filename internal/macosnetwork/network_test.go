package macosnetwork

import "testing"

func TestParseNetworkInfo(t *testing.T) {
	got := parseNetworkInfo("DHCP Configuration\nIP address: 192.168.1.20\nSubnet mask: 255.255.255.0\nRouter: 192.168.1.1\n")
	if got.IPv4 != "192.168.1.20" || got.SubnetMask != "255.255.255.0" || got.Router != "192.168.1.1" {
		t.Fatalf("parseNetworkInfo() = %#v", got)
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
