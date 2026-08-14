//go:build !darwin

package macosipv6

import (
	"fmt"
	"net"
)

func sendRouterWithdrawal(interfaceName string, mac net.HardwareAddr, prefix, dns net.IP) error {
	return fmt.Errorf("IPv6 router withdrawal is only supported on macOS")
}
