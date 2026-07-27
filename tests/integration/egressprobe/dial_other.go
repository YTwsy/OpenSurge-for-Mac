//go:build !darwin

package main

import (
	"fmt"
	"net"
	"time"
)

func dialDirect(target, interfaceName, resolverAddress string, timeout time.Duration) (net.Conn, error) {
	if interfaceName != "" || resolverAddress != "" {
		return nil, fmt.Errorf("binding the controlled proxy to an upstream interface is only supported on Darwin")
	}
	return net.DialTimeout("tcp", target, timeout)
}
