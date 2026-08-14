package ipv6packet

import (
	"net"
	"net/netip"
	"testing"
)

func TestMessageRoundTripPreservesIdentityAndPacket(t *testing.T) {
	mac, _ := net.ParseMAC("aa:bb:cc:dd:ee:01")
	packet := testIPv6Packet("fdfe:dcba:9878::21", "fdfe:dcba:9876::99")
	encoded, err := EncodeMessage(MessageInbound, mac, packet)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMessage(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Type != MessageInbound || decoded.MAC.String() != mac.String() || string(decoded.Packet) != string(packet) {
		t.Fatalf("decoded message mismatch: %#v", decoded)
	}
}

func TestMessageRejectsMalformedFrames(t *testing.T) {
	if _, err := EncodeMessage(99, nil, nil); err == nil {
		t.Fatal("unsupported message type was accepted")
	}
	if _, err := EncodeMessage(MessageInbound, net.HardwareAddr{1, 2}, nil); err == nil {
		t.Fatal("short MAC was accepted")
	}
	if _, err := DecodeMessage([]byte("short")); err == nil {
		t.Fatal("short frame was accepted")
	}
}

func TestParseInboundFrameCapturesOnlyOffLinkClientIPv6(t *testing.T) {
	gatewayMAC, _ := net.ParseMAC("02:00:00:00:00:01")
	clientMAC, _ := net.ParseMAC("02:00:00:00:00:21")
	frame := testEthernetFrame(gatewayMAC, clientMAC, testIPv6Packet("fdfe:dcba:9878::21", "fdfe:dcba:9876::99"))
	packet, source, mac, ok := parseInboundFrame(frame, gatewayMAC, 1500)
	if !ok || source != netip.MustParseAddr("fdfe:dcba:9878::21") || mac.String() != clientMAC.String() || len(packet) != 40 {
		t.Fatalf("off-link frame = ok %v, source %v, mac %v, packet %d", ok, source, mac, len(packet))
	}

	local := testEthernetFrame(gatewayMAC, clientMAC, testIPv6Packet("fdfe:dcba:9878::21", configDownstreamGatewayForTest))
	if _, _, _, ok := parseInboundFrame(local, gatewayMAC, 1500); ok {
		t.Fatal("packet addressed to the OpenSurge IPv6 gateway entered the proxy stack")
	}

	spoofedSource := testEthernetFrame(gatewayMAC, clientMAC, testIPv6Packet("2001:db8:ffff::21", "2001:db8::80"))
	if _, _, _, ok := parseInboundFrame(spoofedSource, gatewayMAC, 1500); ok {
		t.Fatal("a source outside the advertised downstream prefix entered the proxy stack")
	}

	notRouted := testEthernetFrame(clientMAC, gatewayMAC, testIPv6Packet("fdfe:dcba:9878::1", "fdfe:dcba:9878::21"))
	if _, _, _, ok := parseInboundFrame(notRouted, gatewayMAC, 1500); ok {
		t.Fatal("outbound frame was accepted as inbound")
	}
}

func TestNeighborTableRejectsIPv6IdentityConflict(t *testing.T) {
	table := newNeighborTable()
	ip := netip.MustParseAddr("fdfe:dcba:9878::21")
	first, _ := net.ParseMAC("02:00:00:00:00:21")
	second, _ := net.ParseMAC("02:00:00:00:00:22")
	if !table.Observe(ip, first) || table.Observe(ip, second) {
		t.Fatal("neighbor identity conflict did not fail closed")
	}
	got, ok := table.Lookup(ip)
	if !ok || got.String() != first.String() {
		t.Fatalf("neighbor identity changed after conflict: %v, %v", got, ok)
	}
}

const configDownstreamGatewayForTest = "fdfe:dcba:9878::1"

func testIPv6Packet(source, destination string) []byte {
	packet := make([]byte, 40)
	packet[0] = 0x60
	copy(packet[8:24], netip.MustParseAddr(source).AsSlice())
	copy(packet[24:40], netip.MustParseAddr(destination).AsSlice())
	return packet
}

func testEthernetFrame(destination, source net.HardwareAddr, packet []byte) []byte {
	frame := make([]byte, ethernetHeaderSize+len(packet))
	copy(frame[:6], destination)
	copy(frame[6:12], source)
	frame[12], frame[13] = 0x86, 0xdd
	copy(frame[ethernetHeaderSize:], packet)
	return frame
}
