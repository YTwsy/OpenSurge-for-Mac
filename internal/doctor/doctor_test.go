package doctor

import "testing"

func TestCheckInterfacesDiffer(t *testing.T) {
	check := checkInterfacesDiffer("en0", " en0 ")
	if check.OK {
		t.Fatalf("checkInterfacesDiffer() OK = true")
	}
	if check.Message == "" {
		t.Fatalf("checkInterfacesDiffer() missing failure message")
	}

	check = checkInterfacesDiffer("en7", "en0")
	if !check.OK {
		t.Fatalf("checkInterfacesDiffer() OK = false: %s", check.Message)
	}
}

func TestCheckInterfaceIPv4RejectsInvalidIP(t *testing.T) {
	check := checkInterfaceIPv4("en0", "not-an-ip")
	if check.OK {
		t.Fatalf("checkInterfaceIPv4() OK = true")
	}
	if check.Message != "invalid IPv4 address" {
		t.Fatalf("checkInterfaceIPv4() message = %q", check.Message)
	}
}
