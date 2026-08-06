package ipv6packet

import (
	"bytes"
	"fmt"
	"net"
)

const (
	protocolVersion byte = 1
	headerSize           = 12

	MessageHello    byte = 1
	MessageInbound  byte = 2
	MessageOutbound byte = 3
)

var protocolMagic = []byte{'O', 'S', '6', 'P'}

type Message struct {
	Type   byte
	MAC    net.HardwareAddr
	Packet []byte
}

func EncodeMessage(messageType byte, mac net.HardwareAddr, packet []byte) ([]byte, error) {
	if messageType < MessageHello || messageType > MessageOutbound {
		return nil, fmt.Errorf("unsupported IPv6 packet message type %d", messageType)
	}
	if len(packet) > 65535-headerSize {
		return nil, fmt.Errorf("IPv6 packet message is too large: %d", len(packet))
	}
	out := make([]byte, headerSize+len(packet))
	copy(out[:4], protocolMagic)
	out[4] = protocolVersion
	out[5] = messageType
	if len(mac) != 0 {
		if len(mac) != 6 {
			return nil, fmt.Errorf("IPv6 packet sideband MAC must contain 6 bytes")
		}
		copy(out[6:12], mac)
	}
	copy(out[headerSize:], packet)
	return out, nil
}

func DecodeMessage(data []byte) (Message, error) {
	if len(data) < headerSize {
		return Message{}, fmt.Errorf("IPv6 packet message is shorter than %d bytes", headerSize)
	}
	if !bytes.Equal(data[:4], protocolMagic) {
		return Message{}, fmt.Errorf("IPv6 packet message has invalid magic")
	}
	if data[4] != protocolVersion {
		return Message{}, fmt.Errorf("unsupported IPv6 packet protocol version %d", data[4])
	}
	if data[5] < MessageHello || data[5] > MessageOutbound {
		return Message{}, fmt.Errorf("unsupported IPv6 packet message type %d", data[5])
	}
	mac := append(net.HardwareAddr(nil), data[6:12]...)
	return Message{Type: data[5], MAC: mac, Packet: append([]byte(nil), data[headerSize:]...)}, nil
}
