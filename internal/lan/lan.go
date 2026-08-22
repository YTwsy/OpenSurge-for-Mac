// Package lan derives IPv4 subnet facts from the gateway LAN host address and a
// configured prefix length. It exists so config validation, device policy, and
// the control API agree on what "same subnet" means instead of each assuming a
// /24, which silently breaks NAT and TUN route exclusion on wider LANs.
package lan

import (
	"fmt"
	"net"
)

const (
	// DefaultPrefixLen keeps configs written before gateway.lan_prefix_len
	// existed on their original /24 assumption.
	DefaultPrefixLen = 24
	// MinPrefixLen allows the private /8 ranges operators actually deploy.
	MinPrefixLen = 8
	// MaxPrefixLen stops at the smallest subnet that still has usable hosts
	// besides the gateway itself.
	MaxPrefixLen = 30
)

// Scope is the gateway LAN as configured: the Mac's own LAN host address plus
// the subnet derived from the configured prefix length.
type Scope struct {
	Gateway net.IP
	Network *net.IPNet
}

func PrefixLenOrDefault(prefixLen int) int {
	if prefixLen == 0 {
		return DefaultPrefixLen
	}
	return prefixLen
}

func ValidPrefixLen(prefixLen int) bool {
	return prefixLen >= MinPrefixLen && prefixLen <= MaxPrefixLen
}

func NewScope(gatewayIP string, prefixLen int) (Scope, error) {
	gateway := net.ParseIP(gatewayIP).To4()
	if gateway == nil {
		return Scope{}, fmt.Errorf("gateway LAN address %q must be a valid IPv4 address", gatewayIP)
	}
	length := PrefixLenOrDefault(prefixLen)
	if !ValidPrefixLen(length) {
		return Scope{}, fmt.Errorf("gateway LAN prefix length must be between %d and %d", MinPrefixLen, MaxPrefixLen)
	}
	mask := net.CIDRMask(length, 32)
	return Scope{Gateway: gateway, Network: &net.IPNet{IP: gateway.Mask(mask), Mask: mask}}, nil
}

// String is the CIDR consumed by pf NAT rules and mihomo route exclusion.
func (s Scope) String() string {
	return s.Network.String()
}

func (s Scope) PrefixLen() int {
	ones, _ := s.Network.Mask.Size()
	return ones
}

func (s Scope) Contains(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && s.Network.Contains(v4)
}

// Broadcast is the all-ones host address of the subnet.
func (s Scope) Broadcast() net.IP {
	network := s.Network.IP.To4()
	mask := net.IP(s.Network.Mask).To4()
	out := make(net.IP, net.IPv4len)
	for index := range out {
		out[index] = network[index] | ^mask[index]
	}
	return out
}

// UsableHost reports whether ip can be assigned to a client on this LAN.
func (s Scope) UsableHost(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil || !s.Network.Contains(v4) {
		return false
	}
	return !v4.Equal(s.Network.IP.To4()) && !v4.Equal(s.Broadcast())
}

// HostCount is the number of assignable addresses on this LAN.
func (s Scope) HostCount() int {
	ones, bits := s.Network.Mask.Size()
	return 1<<(bits-ones) - 2
}

// Offset converts a host address into its position inside the subnet. The
// network address is offset 0.
func (s Scope) Offset(ip net.IP) (int, bool) {
	v4 := ip.To4()
	if v4 == nil || !s.Network.Contains(v4) {
		return 0, false
	}
	return int(toUint32(v4) - toUint32(s.Network.IP.To4())), true
}

// HostAt resolves a subnet offset back into an address.
func (s Scope) HostAt(offset int) net.IP {
	return fromUint32(toUint32(s.Network.IP.To4()) + uint32(offset))
}

func toUint32(ip net.IP) uint32 {
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func fromUint32(value uint32) net.IP {
	return net.IPv4(byte(value>>24), byte(value>>16), byte(value>>8), byte(value)).To4()
}
