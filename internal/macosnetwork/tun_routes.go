package macosnetwork

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
)

type RouteSelection struct {
	Interface string `json:"interface"`
	Gateway   string `json:"gateway,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
}

func LookupRoute(ctx context.Context, destination string) (RouteSelection, error) {
	output, err := runCommand(ctx, "/sbin/route", "-n", "get", destination)
	if err != nil {
		return RouteSelection{}, err
	}
	route := parseRouteGet(output)
	if route.Interface == "" {
		return RouteSelection{}, fmt.Errorf("route lookup for %s did not report an interface", destination)
	}
	return RouteSelection{Interface: route.Interface, Gateway: route.Gateway, Prefix: routePrefix(route.Destination, route.Mask)}, nil
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
