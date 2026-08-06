//go:build !darwin

package ipv6packet

import "fmt"

func openBPFPacketDevice(interfaceName string, mtu int) (frameDevice, error) {
	return nil, fmt.Errorf("IPv6 packet takeover requires macOS BPF on an Ethernet interface")
}
