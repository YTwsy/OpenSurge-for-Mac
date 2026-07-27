//go:build darwin

package main

import (
	"context"
	"fmt"
	"net"
	"syscall"
	"time"
)

// IP_BOUND_IF is Darwin's per-socket IPv4 interface binding option. The
// controlled Lab proxy uses it so its own upstream connection cannot re-enter
// the TUN path that the proxy is intended to measure.
const ipBoundIf = 25

func dialDirect(target, interfaceName, resolverAddress string, timeout time.Duration) (net.Conn, error) {
	if interfaceName == "" {
		return net.DialTimeout("tcp", target, timeout)
	}
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, fmt.Errorf("resolve upstream interface %s: %w", interfaceName, err)
	}
	dialer := boundDialer(iface.Index, timeout)
	resolvedTarget, err := resolveIPv4Target(target, resolverAddress, dialer, timeout)
	if err != nil {
		return nil, err
	}
	return dialer.Dial("tcp4", resolvedTarget)
}

func boundDialer(interfaceIndex int, timeout time.Duration) net.Dialer {
	return net.Dialer{
		Timeout: timeout,
		Control: func(_, _ string, raw syscall.RawConn) error {
			var controlErr error
			if err := raw.Control(func(fd uintptr) {
				controlErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, ipBoundIf, interfaceIndex)
			}); err != nil {
				return err
			}
			return controlErr
		},
	}
}

func resolveIPv4Target(target, resolverAddress string, dialer net.Dialer, timeout time.Duration) (string, error) {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return "", err
	}
	if net.ParseIP(host) != nil || resolverAddress == "" {
		return target, nil
	}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "udp4", resolverAddress)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	addresses, err := resolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return "", fmt.Errorf("resolve %s via %s: %w", host, resolverAddress, err)
	}
	if len(addresses) == 0 {
		return "", fmt.Errorf("resolve %s via %s: no IPv4 address", host, resolverAddress)
	}
	return net.JoinHostPort(addresses[0].String(), port), nil
}
