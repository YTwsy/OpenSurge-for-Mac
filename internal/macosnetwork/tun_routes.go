package macosnetwork

import (
	"context"
	"fmt"
	"strings"
)

// GlobalTUNRoute describes a high-confidence full-route TUN candidate. It is
// intentionally interface-level evidence: macOS does not expose a dependable
// mapping from a utun interface back to the owning application.
type GlobalTUNRoute struct {
	Interface    string   `json:"interface"`
	Gateway      string   `json:"gateway,omitempty"`
	Destinations []string `json:"destinations"`
}

type RouteSelection struct {
	Interface string `json:"interface"`
	Gateway   string `json:"gateway,omitempty"`
}

// These numeric destinations are deliberately spread across the public IPv4
// space. Requiring every lookup to select the same utun avoids treating a
// normal split-route VPN or an idle utun interface as a global-route conflict.
var globalTUNProbeDestinations = []string{
	"1.1.1.1",
	"8.8.8.8",
	"64.6.64.6",
	"128.0.0.1",
	"160.0.0.1",
	"208.67.222.222",
	"223.5.5.5",
}

// DetectGlobalTUNRoute returns a high-confidence candidate only when all
// representative public destinations currently select the same utun
// interface. ignoredInterface is the configured OpenSurge TUN device: macOS
// cannot reliably prove ownership, so a matching route is not safe to classify
// as an external blocker. Successful TUN readiness after launch remains the
// authoritative startup check.
func DetectGlobalTUNRoute(ctx context.Context, ignoredInterface string) (GlobalTUNRoute, bool, error) {
	var candidate GlobalTUNRoute
	ignoredInterface = strings.TrimSpace(ignoredInterface)
	for index, destination := range globalTUNProbeDestinations {
		output, err := runCommand(ctx, "/sbin/route", "-n", "get", destination)
		if err != nil {
			return GlobalTUNRoute{}, false, err
		}
		route := parseRouteGet(output)
		if route.Interface == "" {
			return GlobalTUNRoute{}, false, fmt.Errorf("route lookup for %s did not report an interface", destination)
		}
		if !strings.HasPrefix(route.Interface, "utun") {
			return GlobalTUNRoute{}, false, nil
		}
		if index == 0 {
			candidate.Interface = route.Interface
			candidate.Gateway = route.Gateway
		} else if route.Interface != candidate.Interface {
			return GlobalTUNRoute{}, false, nil
		}
		candidate.Destinations = append(candidate.Destinations, destination)
	}
	if candidate.Interface == ignoredInterface {
		return GlobalTUNRoute{}, false, nil
	}
	return candidate, true, nil
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
	return RouteSelection{Interface: route.Interface, Gateway: route.Gateway}, nil
}

type routeGetResult struct {
	Interface string
	Gateway   string
}

func parseRouteGet(output string) routeGetResult {
	var result routeGetResult
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "interface":
			result.Interface = strings.TrimSpace(value)
		case "gateway":
			result.Gateway = strings.TrimSpace(value)
		}
	}
	return result
}
