package macosnetwork

import (
	"encoding/binary"
	"testing"
)

func TestParseDHCPOffer(t *testing.T) {
	const xid = uint32(0x11223344)
	packet := make([]byte, 240)
	packet[0] = 2
	binary.BigEndian.PutUint32(packet[4:8], xid)
	copy(packet[236:240], []byte{99, 130, 83, 99})
	packet = append(packet, 53, 1, 2, 54, 4, 192, 168, 1, 1, 255)
	server, ok := parseDHCPOffer(packet, xid)
	if !ok || server != "192.168.1.1" {
		t.Fatalf("parseDHCPOffer() = %q, %t", server, ok)
	}
	if _, ok := parseDHCPOffer(packet, xid+1); ok {
		t.Fatal("accepted wrong transaction")
	}
}
