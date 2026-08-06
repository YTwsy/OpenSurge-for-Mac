package macosipv6

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"

	"open-mihomo-gateway/internal/config"
)

func TestHasDefaultRouteOnInterface(t *testing.T) {
	routes := "default fe80::1%en0 UGcg en0\n::/0 fe80::1%en7 UGcg en7 42\n"
	if !hasDefaultRouteOnInterface(routes, "en0") {
		t.Fatal("IPv6 default route was not detected")
	}
	if !hasDefaultRouteOnInterface(routes, "en7") {
		t.Fatal("IPv6 default route before expiry column was not detected")
	}
	if hasDefaultRouteOnInterface(routes, "en9") {
		t.Fatal("IPv6 default route was detected on the wrong interface")
	}
}

func TestNativeAvailableRequiresAddressAndDefaultRoute(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.UpstreamInterface = "en0"
	manager := New(cfg)
	manager.byName = func(name string) (*net.Interface, error) { return &net.Interface{Name: name}, nil }
	manager.addrs = func(*net.Interface) ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)}}, nil
	}
	manager.output = func(name string, args ...string) ([]byte, error) {
		if name != "/usr/sbin/netstat" {
			t.Fatalf("command = %q", name)
		}
		return []byte("default fe80::1%en7 UGcg en7\n"), nil
	}
	available, err := manager.NativeAvailable()
	if err != nil {
		t.Fatalf("NativeAvailable() error = %v", err)
	}
	if available {
		t.Fatal("IPv6 address without a default route on the upstream interface was treated as available")
	}
}

func TestNativeAvailableRejectsUniqueLocalAddress(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.UpstreamInterface = "en0"
	manager := New(cfg)
	manager.byName = func(name string) (*net.Interface, error) { return &net.Interface{Name: name}, nil }
	manager.addrs = func(*net.Interface) ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("fd12:3456:789a::1"), Mask: net.CIDRMask(64, 128)}}, nil
	}
	manager.output = func(string, ...string) ([]byte, error) {
		t.Fatal("a ULA must not trigger the public IPv6 default-route probe")
		return nil, nil
	}
	available, err := manager.NativeAvailable()
	if err != nil || available {
		t.Fatalf("NativeAvailable() = %v, %v", available, err)
	}
}

func TestNativeAvailableReturnsRouteProbeErrorAfterUsableAddress(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.UpstreamInterface = "en0"
	manager := New(cfg)
	manager.byName = func(name string) (*net.Interface, error) { return &net.Interface{Name: name}, nil }
	manager.addrs = func(*net.Interface) ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)}}, nil
	}
	manager.output = func(string, ...string) ([]byte, error) { return nil, errors.New("route probe failed") }
	if available, err := manager.NativeAvailable(); err == nil || available {
		t.Fatalf("NativeAvailable() = %v, %v", available, err)
	}
}

func TestGatewayAliasCommandsAndWithdrawal(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "lan0"
	manager := New(cfg)
	var commands [][]string
	manager.runCommand = func(_ context.Context, name string, args ...string) error {
		commands = append(commands, append([]string{name}, args...))
		return nil
	}
	manager.output = func(name string, args ...string) ([]byte, error) {
		return []byte("\tinet6 fdfe:dcba:9878::1 prefixlen 64 secured\n"), nil
	}
	mac, _ := net.ParseMAC("02:00:00:00:00:01")
	manager.byName = func(name string) (*net.Interface, error) {
		return &net.Interface{Name: name, HardwareAddr: mac}, nil
	}
	manager.addrs = func(*net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP(config.DownstreamIPv6Gateway), Mask: net.CIDRMask(64, 128)},
			&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
		}, nil
	}
	manager.withdraw = func(name string, gotMAC net.HardwareAddr, prefix, dns net.IP) error {
		if name != "lan0" || gotMAC.String() != mac.String() || !prefix.Equal(net.ParseIP("fdfe:dcba:9878::")) || !dns.Equal(net.ParseIP("fe80::1")) {
			t.Fatalf("withdraw args = %s, %s, %s, %s", name, gotMAC, prefix, dns)
		}
		return nil
	}
	if err := manager.AddGateway(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.RemoveGateway(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Withdraw(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"/sbin/ifconfig", "lan0", "inet6", "fdfe:dcba:9878::1", "prefixlen", "64", "alias"},
		{"/sbin/ifconfig", "lan0", "inet6", "fdfe:dcba:9878::1", "-alias"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
}

func TestWithdrawRequiresLinkLocalRDNSSAddress(t *testing.T) {
	cfg := config.Default()
	cfg.Gateway.Interface = "lan0"
	manager := New(cfg)
	manager.byName = func(string) (*net.Interface, error) { return &net.Interface{Name: "lan0"}, nil }
	manager.addrs = func(*net.Interface) ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP(config.DownstreamIPv6Gateway), Mask: net.CIDRMask(64, 128)}}, nil
	}
	manager.withdraw = func(string, net.HardwareAddr, net.IP, net.IP) error {
		t.Fatal("withdrawal was sent without the advertised link-local RDNSS address")
		return nil
	}
	if err := manager.Withdraw(context.Background()); err == nil {
		t.Fatal("Withdraw() accepted an interface without link-local IPv6")
	}
}

func TestGatewayAddressStateWaitsForDADAndRejectsDuplicate(t *testing.T) {
	if ready, duplicated := gatewayAddressState("inet6 fdfe:dcba:9878::1 prefixlen 64 tentative secured\n", config.DownstreamIPv6Gateway); ready || duplicated {
		t.Fatalf("tentative state = ready %v, duplicated %v", ready, duplicated)
	}
	if ready, duplicated := gatewayAddressState("inet6 fdfe:dcba:9878::1 prefixlen 64 secured\n", config.DownstreamIPv6Gateway); !ready || duplicated {
		t.Fatalf("ready state = ready %v, duplicated %v", ready, duplicated)
	}
	if ready, duplicated := gatewayAddressState("inet6 fdfe:dcba:9878::1 prefixlen 64 duplicated\n", config.DownstreamIPv6Gateway); ready || !duplicated {
		t.Fatalf("duplicated state = ready %v, duplicated %v", ready, duplicated)
	}
}

func TestCheckGatewayAvailableRejectsUnownedAlias(t *testing.T) {
	cfg := config.Default()
	manager := New(cfg)
	manager.byName = func(name string) (*net.Interface, error) { return &net.Interface{Name: name}, nil }
	manager.addrs = func(*net.Interface) ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("fdfe:dcba:9878::1"), Mask: net.CIDRMask(64, 128)}}, nil
	}
	if err := manager.CheckGatewayAvailable(); err == nil {
		t.Fatal("unowned OpenSurge IPv6 gateway alias was accepted")
	}
}

func TestRemoveGatewayIsIdempotentWhenAliasAlreadyMissing(t *testing.T) {
	cfg := config.Default()
	manager := New(cfg)
	manager.byName = func(name string) (*net.Interface, error) { return &net.Interface{Name: name}, nil }
	manager.addrs = func(*net.Interface) ([]net.Addr, error) { return nil, nil }
	manager.runCommand = func(context.Context, string, ...string) error {
		t.Fatal("RemoveGateway ran ifconfig for an already absent alias")
		return nil
	}
	if err := manager.RemoveGateway(context.Background()); err != nil {
		t.Fatal(err)
	}
}
