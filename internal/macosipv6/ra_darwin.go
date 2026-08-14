//go:build darwin

package macosipv6

import (
	"fmt"
	"net"
	"syscall"
)

func sendRouterWithdrawal(interfaceName string, mac net.HardwareAddr, prefix, dns net.IP) error {
	payload, err := routerWithdrawalPayload(mac, prefix, dns)
	if err != nil {
		return err
	}
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return err
	}
	fd, err := syscall.Socket(syscall.AF_INET6, syscall.SOCK_RAW, syscall.IPPROTO_ICMPV6)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	for _, option := range []struct{ name, value int }{
		{syscall.IPV6_MULTICAST_IF, iface.Index},
		{syscall.IPV6_MULTICAST_HOPS, 255},
		{syscall.IPV6_UNICAST_HOPS, 255},
		{syscall.IPV6_CHECKSUM, 2},
	} {
		if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, option.name, option.value); err != nil {
			return err
		}
	}
	destination := &syscall.SockaddrInet6{ZoneId: uint32(iface.Index)}
	copy(destination.Addr[:], net.ParseIP("ff02::1").To16())
	for attempt := 0; attempt < 3; attempt++ {
		if err := syscall.Sendto(fd, payload, 0, destination); err != nil {
			return fmt.Errorf("send router lifetime zero on %s: %w", interfaceName, err)
		}
	}
	return nil
}
