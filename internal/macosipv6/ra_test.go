package macosipv6

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestRouterWithdrawalPayloadExpiresRoutePrefixAndDNS(t *testing.T) {
	mac, _ := net.ParseMAC("02:00:00:00:00:01")
	prefix := net.ParseIP("fdfe:dcba:9878::")
	dns := net.ParseIP("fdfe:dcba:9878::1")
	payload, err := routerWithdrawalPayload(mac, prefix, dns)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != 80 || payload[0] != 134 || payload[4] != 64 {
		t.Fatalf("unexpected RA header: len=%d type=%d hop=%d", len(payload), payload[0], payload[4])
	}
	if binary.BigEndian.Uint16(payload[6:8]) != 0 || binary.BigEndian.Uint32(payload[28:32]) != 0 || binary.BigEndian.Uint32(payload[32:36]) != 0 {
		t.Fatal("router or prefix lifetime is not zero")
	}
	if got := net.IP(payload[40:56]); !got.Equal(prefix) {
		t.Fatalf("prefix = %s", got)
	}
	if payload[56] != 25 || binary.BigEndian.Uint32(payload[60:64]) != 0 || !net.IP(payload[64:80]).Equal(dns) {
		t.Fatal("RDNSS option does not expire the OpenSurge DNS address")
	}
}

func TestRouterWithdrawalPayloadRejectsNonEthernetMAC(t *testing.T) {
	if _, err := routerWithdrawalPayload(net.HardwareAddr{1, 2}, net.ParseIP("fd00::"), net.ParseIP("fd00::1")); err == nil {
		t.Fatal("short MAC was accepted")
	}
}
