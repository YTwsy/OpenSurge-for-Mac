package macosipv6

import (
	"errors"
	"net"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestHasDefaultRouteOnInterface(t *testing.T) {
	routes := "default fe80::1%en0 UGcg en0\n::1 ::1 UHL lo0\n"
	if !hasDefaultRouteOnInterface(routes, "en0") {
		t.Fatal("IPv6 default route not detected")
	}
	if hasDefaultRouteOnInterface(routes, "en7") {
		t.Fatal("IPv6 default route detected on wrong interface")
	}
}

func TestAddressIP(t *testing.T) {
	for _, addr := range []net.Addr{
		&net.IPNet{IP: net.ParseIP("fd00::1"), Mask: net.CIDRMask(64, 128)},
		&net.IPAddr{IP: net.ParseIP("2001:db8::1")},
		stringAddr("fd00::2/64"),
	} {
		if addressIP(addr) == nil {
			t.Fatalf("addressIP(%v) = nil", addr)
		}
	}
	if got := addressIP(stringAddr("invalid")); got != nil {
		t.Fatalf("addressIP(invalid) = %v", got)
	}
}

func TestNativeAvailableReturnsInterfaceError(t *testing.T) {
	previous := interfaceByName
	t.Cleanup(func() { interfaceByName = previous })
	interfaceByName = func(string) (*net.Interface, error) {
		return nil, errors.New("missing interface")
	}
	if available, err := New(config.Default()).NativeAvailable(); err == nil || available {
		t.Fatalf("NativeAvailable() = %v, %v", available, err)
	}
}

func TestNativeAvailableRequiresGlobalAddressAndDefaultRouteOnUpstream(t *testing.T) {
	previousByName := interfaceByName
	previousAddrs := interfaceAddrs
	previousOutput := commandOutput
	t.Cleanup(func() {
		interfaceByName = previousByName
		interfaceAddrs = previousAddrs
		commandOutput = previousOutput
	})

	interfaceByName = func(name string) (*net.Interface, error) {
		return &net.Interface{Name: name}, nil
	}
	interfaceAddrs = func(*net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
			&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
		}, nil
	}
	commandOutput = func(name string, args ...string) ([]byte, error) {
		if name != "/usr/sbin/netstat" {
			t.Fatalf("command = %q", name)
		}
		return []byte("default fe80::1%en0 UGcg en0\n"), nil
	}

	cfg := config.Default()
	cfg.Gateway.UpstreamInterface = "en0"
	available, err := New(cfg).NativeAvailable()
	if err != nil || !available {
		t.Fatalf("NativeAvailable() = %v, %v", available, err)
	}

	interfaceAddrs = func(*net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
		}, nil
	}
	available, err = New(cfg).NativeAvailable()
	if err != nil || available {
		t.Fatalf("NativeAvailable() with only link-local = %v, %v", available, err)
	}
}

func TestTUNEffectiveRequiresExpectedAddress(t *testing.T) {
	previousByName := interfaceByName
	previousAddrs := interfaceAddrs
	t.Cleanup(func() {
		interfaceByName = previousByName
		interfaceAddrs = previousAddrs
	})
	interfaceByName = func(name string) (*net.Interface, error) {
		return &net.Interface{Name: name}, nil
	}
	interfaceAddrs = func(*net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("fdfe:dcba:9877::1"), Mask: net.CIDRMask(126, 128)},
		}, nil
	}

	cfg := config.Default()
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Always
	if !New(cfg).TUNEffective() {
		t.Fatal("TUNEffective() = false")
	}
	cfg.Transparent.TUNIPv6 = config.TUNIPv6Off
	if New(cfg).TUNEffective() {
		t.Fatal("TUNEffective() = true while disabled")
	}
}

type stringAddr string

func (a stringAddr) Network() string { return "test" }
func (a stringAddr) String() string  { return string(a) }
