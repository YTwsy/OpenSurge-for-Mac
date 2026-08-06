package opensurge_packet

import (
	"bytes"
	"fmt"
	"net"
)

const (
	protocolVersion byte = 1
	headerSize           = 12

	messageHello    byte = 1
	messageInbound  byte = 2
	messageOutbound byte = 3
)

var protocolMagic = []byte{'O', 'S', '6', 'P'}

type packetMessage struct {
	typ    byte
	mac    net.HardwareAddr
	packet []byte
}

func encodeMessage(typ byte, mac net.HardwareAddr, packet []byte) ([]byte, error) {
	if typ < messageHello || typ > messageOutbound {
		return nil, fmt.Errorf("unsupported message type %d", typ)
	}
	out := make([]byte, headerSize+len(packet))
	copy(out[:4], protocolMagic)
	out[4], out[5] = protocolVersion, typ
	if len(mac) != 0 {
		if len(mac) != 6 {
			return nil, fmt.Errorf("sideband MAC must contain 6 bytes")
		}
		copy(out[6:12], mac)
	}
	copy(out[headerSize:], packet)
	return out, nil
}

func decodeMessage(data []byte) (packetMessage, error) {
	if len(data) < headerSize || !bytes.Equal(data[:4], protocolMagic) {
		return packetMessage{}, fmt.Errorf("invalid OpenSurge packet message")
	}
	if data[4] != protocolVersion || data[5] < messageHello || data[5] > messageOutbound {
		return packetMessage{}, fmt.Errorf("unsupported OpenSurge packet protocol")
	}
	return packetMessage{
		typ:    data[5],
		mac:    append(net.HardwareAddr(nil), data[6:12]...),
		packet: append([]byte(nil), data[headerSize:]...),
	}, nil
}
