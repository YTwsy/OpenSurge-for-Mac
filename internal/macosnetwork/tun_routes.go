package macosnetwork

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"open-mihomo-gateway/internal/config"
)

type RouteSelection struct {
	Interface string `json:"interface"`
	Gateway   string `json:"gateway,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
}

func LookupRoute(ctx context.Context, destination string) (RouteSelection, error) {
	args := []string{"-n", "get"}
	if addr, err := netip.ParseAddr(destination); err == nil && addr.Is6() {
		args = append(args, "-inet6")
	}
	output, err := runCommand(ctx, "/sbin/route", append(args, destination)...)
	if err != nil {
		return RouteSelection{}, err
	}
	route := parseRouteGet(output)
	if route.Interface == "" {
		return RouteSelection{}, fmt.Errorf("route lookup for %s did not report an interface", destination)
	}
	return RouteSelection{Interface: route.Interface, Gateway: route.Gateway, Prefix: routePrefix(route.Destination, route.Mask)}, nil
}

// VerifyMacTUNRoutes is a bounded startup/Doctor check, never a status poll.
// Check both ends of the synthetic /64 as well as a public IPv6 destination.
func VerifyMacTUNRoutes(ctx context.Context, device string) error {
	if device == "" {
		return fmt.Errorf("Mac TUN device is missing")
	}
	for _, destination := range []string{config.LocalSystemDNSServer, "198.18.0.2", "fdfe:dcba:9876::2", "fdfe:dcba:9876:0:ffff:ffff:ffff:ffff", "2001:4860:4860::8888"} {
		route, err := LookupRoute(ctx, destination)
		if err != nil {
			return err
		}
		if route.Interface != device {
			return fmt.Errorf("Mac TUN route for %s selects %s, expected %s", destination, route.Interface, device)
		}
	}
	return nil
}

type routeGetResult struct {
	Interface   string
	Gateway     string
	Destination string
	Mask        string
}

func parseRouteGet(output string) routeGetResult {
	var result routeGetResult
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "destination":
			result.Destination = strings.TrimSpace(value)
		case "mask":
			result.Mask = strings.TrimSpace(value)
		case "interface":
			result.Interface = strings.TrimSpace(value)
		case "gateway":
			result.Gateway = strings.TrimSpace(value)
		}
	}
	return result
}

func routePrefix(destination, mask string) string {
	address, addressErr := netip.ParseAddr(strings.TrimSpace(destination))
	maskAddress, maskErr := netip.ParseAddr(strings.TrimSpace(mask))
	if addressErr != nil || maskErr != nil || address.BitLen() != maskAddress.BitLen() {
		return ""
	}
	maskBytes := maskAddress.AsSlice()
	prefixBits := 0
	zeroSeen := false
	for _, value := range maskBytes {
		for bit := 7; bit >= 0; bit-- {
			set := value&(1<<bit) != 0
			if zeroSeen && set {
				return ""
			}
			if set {
				prefixBits++
			} else {
				zeroSeen = true
			}
		}
	}
	return netip.PrefixFrom(address, prefixBits).Masked().String()
}
