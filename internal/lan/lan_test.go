package lan

import (
	"net"
	"testing"
)

func TestNewScopeDefaultsToSlash24(t *testing.T) {
	scope, err := NewScope("192.168.50.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := scope.String(); got != "192.168.50.0/24" {
		t.Fatalf("scope = %s", got)
	}
	if !scope.Contains(net.ParseIP("192.168.50.200")) || scope.Contains(net.ParseIP("192.168.51.200")) {
		t.Fatalf("unexpected membership for %s", scope)
	}
}

func TestScopeHonorsWiderPrefix(t *testing.T) {
	scope, err := NewScope("192.168.50.1", 22)
	if err != nil {
		t.Fatal(err)
	}
	if got := scope.String(); got != "192.168.48.0/22" {
		t.Fatalf("scope = %s", got)
	}
	if !scope.Contains(net.ParseIP("192.168.51.200")) {
		t.Fatalf("%s should contain 192.168.51.200", scope)
	}
	if got := scope.Broadcast().String(); got != "192.168.51.255" {
		t.Fatalf("broadcast = %s", got)
	}
	if scope.UsableHost(scope.Broadcast()) || scope.UsableHost(net.ParseIP("192.168.48.0")) {
		t.Fatalf("%s network and broadcast must not be usable hosts", scope)
	}
}

func TestScopeOffsetsRoundTrip(t *testing.T) {
	scope, err := NewScope("192.168.1.20", 28)
	if err != nil {
		t.Fatal(err)
	}
	offset, ok := scope.Offset(net.ParseIP("192.168.1.20"))
	if !ok || offset != 4 {
		t.Fatalf("offset = %d, ok = %v", offset, ok)
	}
	if got := scope.HostAt(offset).String(); got != "192.168.1.20" {
		t.Fatalf("HostAt(%d) = %s", offset, got)
	}
	if got := scope.HostCount(); got != 14 {
		t.Fatalf("HostCount() = %d", got)
	}
}

func TestNewScopeRejectsUnsupportedPrefix(t *testing.T) {
	if _, err := NewScope("192.168.1.20", 31); err == nil {
		t.Fatal("/31 has no usable host addresses and must be rejected")
	}
	if _, err := NewScope("not-an-ip", 24); err == nil {
		t.Fatal("a non-IPv4 gateway address must be rejected")
	}
}
